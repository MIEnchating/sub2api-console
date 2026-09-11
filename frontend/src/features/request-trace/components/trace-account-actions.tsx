import { ContentLoading } from "@/components/content-loading";
import { QueryErrorToast } from "@/components/query-error-toast";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, LoaderCircle, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { api, type AccountControlAction } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import { accountPoolState } from "@/features/accounts/lib/account-pool";
import { notifyOperationError } from "@/lib/operation-feedback";
import { terminalRefreshKeys } from "@/lib/task-refresh";
import { taskIsPending, taskPollInterval, taskStopsPolling } from "@/lib/task-state";

type TraceControlAction = Extract<AccountControlAction, "fuse" | "recover" | "resume">;

const actionLabels: Record<TraceControlAction, string> = {
  fuse: "手动熔断",
  recover: "解除熔断",
  resume: "恢复调度",
};

export function TraceAccountActions(props: { accountId: string }): React.ReactElement {
  const queryClient = useQueryClient();
  const [confirmation, setConfirmation] = useState<TraceControlAction | null>(null);
  const [taskId, setTaskId] = useState<string | null>(null);
  const refreshedTask = useRef<string | null>(null);
  const validId = /^[1-9]\d*$/.test(props.accountId);
  const account = useQuery({
    queryKey: ["account-detail", props.accountId],
    queryFn: () => api.account(props.accountId),
    enabled: validId,
    retry: false,
  });
  const task = useQuery({
    queryKey: ["account-scheduling", props.accountId, taskId],
    queryFn: () => api.task(taskId!),
    enabled: taskId !== null,
    refetchInterval: taskPollInterval,
  });
  const control = useMutation({
    mutationFn: (action: TraceControlAction) => api.setAccountControl(props.accountId, action),
    onSuccess: (created) => {
      setTaskId(created.id);
      setConfirmation(null);
    },
    onError: (error) => notifyOperationError(error, "账号处置启动失败"),
  });

  useEffect(() => {
    const completed = task.data;
    if (!completed || !taskStopsPolling(completed) || refreshedTask.current === completed.id)
      return;
    refreshedTask.current = completed.id;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["account-detail", props.accountId] }),
      ...terminalRefreshKeys("account-scheduling", completed).map((queryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
    ]);
    if (completed.status === "succeeded") toast.success("账号处置完成");
    else if (completed.status === "cancelled") toast.info(completed.message || "账号处置已取消");
    else toast.error(completed.message || "账号处置失败");
  }, [props.accountId, queryClient, task.data]);

  if (!validId) {
    return <p className="text-muted-foreground text-xs">账号 ID 未记录或无效，无法处置</p>;
  }

  const current = account.data;
  const state = current ? accountPoolState(current).value : null;
  const fused = state === "fused";
  const paused = state === "paused";
  const pending = control.isPending || taskIsPending(taskId, task);
  const disabled =
    pending ||
    account.isFetching ||
    account.isError ||
    !current ||
    current.id !== props.accountId ||
    current.manual_priority != null ||
    state === "excluded";
  const recoverAction = fused || state === "cost_blocked" ? "recover" : "resume";
  const label = confirmation ? actionLabels[confirmation] : "账号处置";
  const identity = current ? `${current.name}（ID：${current.id}）` : `账号 ${props.accountId}`;
  const description =
    confirmation === "fuse"
      ? `手动熔断“${identity}”后，该账号会立即停止接收流量，并持续保持熔断状态，直到手动解除。`
      : `确认恢复“${identity}”的调度？恢复后该账号可重新接收流量，后续仍受调度策略约束。`;

  return (
    <div className="mt-2 grid min-w-0 gap-2">
      <div
        className="flex flex-wrap items-center gap-2"
        role="group"
        aria-label={`账号 ${props.accountId} 处置`}
      >
        <Button
          variant="outline"
          disabled={disabled || fused || paused}
          onClick={() => setConfirmation("fuse")}
        >
          <Ban aria-hidden="true" />
          手动熔断
        </Button>
        <Button
          variant="outline"
          disabled={disabled || (!fused && !paused && current?.schedulable !== false)}
          onClick={() => setConfirmation(recoverAction)}
        >
          <RefreshCw aria-hidden="true" />
          {fused ? actionLabels.recover : actionLabels.resume}
        </Button>
      </div>
      {account.isLoading ? <ContentLoading label="正在读取账号状态" compact /> : null}
      {account.isError ? (
        <div className="text-destructive flex flex-wrap items-center gap-2 text-xs">
          <QueryErrorToast error={account.error} fallback="账号状态读取失败，请重试" />
          <Button
            variant="ghost"
            onClick={() => void account.refetch()}
            disabled={account.isFetching}
          >
            <RefreshCw aria-hidden="true" />
            重试
          </Button>
        </div>
      ) : null}
      {current?.manual_priority != null ? (
        <p className="text-muted-foreground text-xs">请先取消人工优先位</p>
      ) : null}
      {state === "excluded" ? (
        <p className="text-muted-foreground text-xs">请先在账号管理恢复管控</p>
      ) : null}
      {pending ? (
        <p role="status" className="text-muted-foreground flex items-center gap-1 text-xs">
          <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
          {task.isError ? "正在重新读取任务状态" : "账号处置执行中"}
        </p>
      ) : null}
      {!pending && task.data ? (
        <p role="status" className="text-muted-foreground break-words text-xs">
          {task.data.message}
        </p>
      ) : null}
      <ConfirmActionDialog
        open={confirmation !== null}
        title={label}
        description={description}
        confirmLabel={`确认${label}`}
        pending={control.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation && !disabled) control.mutate(confirmation);
        }}
      />
    </div>
  );
}
