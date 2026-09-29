import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { api, type UpstreamGroupChange, type UpstreamGroupBindingAudit } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { notifyTaskResult } from "@/lib/task-result-feedback";

function boundAccountIDs(audit: UpstreamGroupBindingAudit, change: UpstreamGroupChange): string[] {
  const item = audit.items.find(
    (candidate) =>
      candidate.upstream_id === change.upstream_id &&
      candidate.group_id === change.group_id &&
      candidate.status === "missing",
  );
  return [...new Set(item?.accounts.map((account) => account.id) ?? [])].sort();
}

export function RemovedGroupAccountsDialog(props: {
  change: UpstreamGroupChange;
  expectedAccountIDs: string[];
  onClose: () => void;
  onFinished: () => void;
}) {
  const queryClient = useQueryClient();
  const [taskID, setTaskID] = useState<string | null>(null);
  const completedTaskID = useRef<string | null>(null);
  const audit = useQuery({
    queryKey: ["upstream-group-binding-audit"],
    queryFn: api.upstreamGroupBindingAudit,
    refetchOnMount: "always",
    retry: false,
  });
  const currentIDs = audit.data ? boundAccountIDs(audit.data, props.change) : [];
  const expectedIDs = [...props.expectedAccountIDs].sort();
  const unchanged =
    currentIDs.length === expectedIDs.length &&
    currentIDs.every((id, index) => id === expectedIDs[index]);
  const batchIDs = currentIDs.slice(0, 50);
  const preview = useQuery({
    queryKey: ["account-delete-batch-preview", batchIDs],
    queryFn: () => api.accountDeleteBatchPreview(batchIDs),
    enabled: unchanged && !audit.isError && batchIDs.length > 0 && !taskID,
    retry: false,
  });
  const deletion = useMutation({
    mutationFn: async () => {
      const latestAudit = await api.upstreamGroupBindingAudit();
      const latestIDs = boundAccountIDs(latestAudit, props.change);
      if (
        latestIDs.length !== currentIDs.length ||
        latestIDs.some((id, index) => id !== currentIDs[index])
      ) {
        throw new Error("分组绑定账号已变化，请关闭弹窗后重新查看影响范围");
      }
      const latestPreview = await api.accountDeleteBatchPreview(batchIDs);
      if (JSON.stringify(latestPreview) !== JSON.stringify(preview.data)) {
        throw new Error("账号删除影响范围已变化，请刷新预览后重新确认");
      }
      return api.deleteAccounts(latestPreview);
    },
    onSuccess: (task) => setTaskID(task.id),
    onError: (error) => notifyOperationError(error, "绑定账号删除启动失败"),
  });
  const task = useQuery({
    queryKey: ["account-delete-batch", taskID],
    queryFn: () => api.task(taskID!),
    enabled: Boolean(taskID),
    refetchInterval: taskPollInterval,
    retry: false,
  });
  const pending =
    deletion.isPending || (Boolean(taskID) && !task.error && !taskStopsPolling(task.data));
  useEffect(() => {
    if (!task.data || !taskStopsPolling(task.data) || completedTaskID.current === task.data.id)
      return;
    completedTaskID.current = task.data.id;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["upstream-group-binding-audit"] }),
      queryClient.invalidateQueries({ queryKey: ["accounts"] }),
      queryClient.invalidateQueries({ queryKey: ["upstreams"] }),
    ]);
    notifyTaskResult(task.data, "绑定账号删除");
    props.onFinished();
  }, [task.data, queryClient, props]);

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) props.onClose();
      }}
    >
      <DialogContent width="wide" height="large" showCloseButton={!pending}>
        <DialogHeader>
          <DialogTitle>删除「{props.change.group_name}」的绑定账号</DialogTitle>
          <DialogDescription>
            按稳定分组 ID 核对账号后删除管理平台账号、Console 记录及可确认的独占上游
            Key。此操作不可撤销。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-4">
          {audit.isLoading && <ContentLoading label="正在核对分组绑定账号" />}
          {audit.error && (
            <>
              <QueryErrorToast error={audit.error} fallback="分组绑定核对失败" />
              <ContentRetry onRetry={() => void audit.refetch()} pending={audit.isFetching} />
            </>
          )}
          {audit.data && !taskID && (!unchanged || currentIDs.length === 0) && (
            <p className="text-muted-foreground text-sm">
              分组绑定已变化，请关闭后重新查看统计变化。
            </p>
          )}
          {unchanged && currentIDs.length > 0 && !taskID && preview.isLoading && (
            <ContentLoading label="正在读取账号删除影响范围" />
          )}
          {preview.error && !taskID && (
            <>
              <QueryErrorToast error={preview.error} fallback="账号删除影响范围读取失败" />
              <ContentRetry onRetry={() => void preview.refetch()} pending={preview.isFetching} />
            </>
          )}
          {unchanged && preview.data && !taskID && !deletion.isPending && (
            <>
              <p className="text-sm">
                当前绑定 {currentIDs.length} 个账号，本次将删除 {batchIDs.length} 个，其中{" "}
                {preview.data.upstream_key_count} 个会连带删除独占上游 Key。
                {currentIDs.length > 50 ? "剩余账号请在本次完成后继续处理。" : ""}
              </p>
              <div className="min-h-0 max-h-64 divide-y overflow-y-auto rounded-md border">
                {preview.data.accounts.map((account) => (
                  <div key={account.account_id} className="grid gap-1 px-3 py-2 text-sm">
                    <span>
                      {account.account_name}（ID {account.account_id}）
                    </span>
                    <span className="text-muted-foreground text-xs">
                      {account.binding
                        ? `连带删除上游 Key：${account.binding.upstream_key_name || account.binding.upstream_key_id}`
                        : "仅删除管理平台账号和本地记录"}
                    </span>
                  </div>
                ))}
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={props.onClose}>
                  取消
                </Button>
                <Button
                  variant="destructive"
                  disabled={
                    preview.isFetching || audit.isFetching || preview.isError || audit.isError
                  }
                  onClick={() => deletion.mutate()}
                >
                  <Trash2 aria-hidden="true" />
                  确认删除 {batchIDs.length} 个账号
                </Button>
              </div>
            </>
          )}
          {deletion.isPending && <TaskStartupState message="正在创建账号删除任务" />}
          {taskID && !task.data && !task.error && (
            <TaskStartupState message="正在读取账号删除任务" />
          )}
          {task.error && (
            <>
              <QueryErrorToast error={task.error} fallback="账号删除任务读取失败" />
              <ContentRetry onRetry={() => void task.refetch()} pending={task.isFetching} />
            </>
          )}
          {task.data && !taskStopsPolling(task.data) && (
            <TaskProgressState
              taskId={task.data.id}
              message={task.data.message}
              progress={task.data.progress}
            />
          )}
          {task.data && taskStopsPolling(task.data) && (
            <div className="grid gap-3">
              <p role="status" className="text-sm">
                {task.data.message}
              </p>
              <Button variant="outline" onClick={props.onClose}>
                关闭
              </Button>
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
