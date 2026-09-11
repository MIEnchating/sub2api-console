import { QueryErrorToast } from "@/components/query-error-toast";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, LoaderCircle } from "lucide-react";
import { toast } from "sonner";

import { api, type AccountStatus, type TaskSummary } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyProbeTaskResult } from "@/lib/probe-task-feedback";
import { terminalRefreshKeys } from "@/lib/task-refresh";
import { taskIsPending, taskPollInterval, taskStopsPolling } from "@/lib/task-state";

export function AccountBatchProbeDialog(props: {
  open: boolean;
  accounts: AccountStatus[];
  onOpenChange: (open: boolean) => void;
  onPendingChange: (pending: boolean) => void;
  onStarted: () => void;
}): React.ReactElement {
  const queryClient = useQueryClient();
  const [taskId, setTaskId] = useState<string | null>(null);
  const refreshedTask = useRef<string | null>(null);
  const eligible = props.accounts.filter((account) => account.manual_priority == null);
  const skipped = props.accounts.length - eligible.length;
  const task = useQuery({
    queryKey: ["account-batch-probe", taskId],
    queryFn: () => api.task(taskId!),
    enabled: taskId !== null,
    refetchInterval: taskPollInterval,
  });
  const run = useMutation({
    mutationFn: (accountIds: string[]) => api.runActiveProbe({ account_ids: accountIds }),
    onSuccess: (created) => {
      setTaskId(created.id);
      queryClient.setQueryData(["task", created.id], created);
      queryClient.setQueryData<TaskSummary[]>(["tasks"], (current) =>
        [
          {
            id: created.id,
            skill: created.skill,
            operation: created.operation,
            status: created.status,
            progress: created.progress,
            message: created.message,
            created_at: created.created_at,
            updated_at: created.updated_at,
            system_info: true as const,
          },
          ...(current ?? []).filter((item) => item.id !== created.id),
        ].slice(0, 20),
      );
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      props.onOpenChange(false);
      props.onStarted();
      toast.success("批量探活任务已转入后台");
    },
  });
  const pending = run.isPending || taskIsPending(taskId, task);
  useEffect(() => {
    props.onPendingChange(pending);
  }, [pending, props.onPendingChange]);
  useEffect(() => {
    if (props.open) run.reset();
  }, [props.open]);
  useEffect(() => {
    const completed = task.data;
    if (!completed || !taskStopsPolling(completed) || refreshedTask.current === completed.id)
      return;
    refreshedTask.current = completed.id;
    void Promise.all([
      ...terminalRefreshKeys("active-probe", completed).map((queryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
      queryClient.invalidateQueries({ queryKey: ["account-detail"] }),
      queryClient.invalidateQueries({ queryKey: ["tasks"] }),
    ]);
    notifyProbeTaskResult(completed);
  }, [queryClient, task.data]);

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!run.isPending) props.onOpenChange(open);
      }}
    >
      <DialogContent width="medium" showCloseButton={!run.isPending}>
        <DialogHeader>
          <DialogTitle>批量探活</DialogTitle>
          <DialogDescription>
            本次探活 {eligible.length} 个账号，使用各账号已配置的探活模型。
            {skipped > 0 ? `已跳过 ${skipped} 个人工优先位账号。` : ""}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid min-w-0 gap-3">
          <ul aria-label="本次探活账号" className="max-h-64 min-w-0 divide-y overflow-y-auto">
            {eligible.map((account) => (
              <li key={account.id} className="py-2 text-sm break-words">
                <span className="font-medium">{account.name}</span>
                <span className="text-muted-foreground ml-2">ID {account.id}</span>
              </li>
            ))}
          </ul>
          {eligible.length === 0 ? (
            <p role="alert" className="text-muted-foreground text-sm">
              没有可探活账号，请选择非人工优先位账号。
            </p>
          ) : null}
          {eligible.length > 100 ? (
            <p role="alert" className="text-destructive text-sm">
              单次最多探活 100 个账号，请减少选择。
            </p>
          ) : null}
          {run.error ? (
            <QueryErrorToast error={run.error} fallback="批量探活启动失败，请重试" />
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button
            variant="outline"
            disabled={run.isPending}
            onClick={() => props.onOpenChange(false)}
          >
            关闭
          </Button>
          <Button
            disabled={pending || eligible.length === 0 || eligible.length > 100}
            onClick={() => run.mutate(eligible.map((account) => account.id))}
          >
            {run.isPending ? (
              <LoaderCircle className="animate-spin" aria-hidden="true" />
            ) : (
              <Activity aria-hidden="true" />
            )}
            {run.isPending ? "正在启动" : "确认探活"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
