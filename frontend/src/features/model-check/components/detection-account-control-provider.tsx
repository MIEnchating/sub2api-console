import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
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
import { notifyOperationError } from "@/lib/operation-feedback";
import { notifyTaskResult } from "@/lib/task-result-feedback";
import { terminalRefreshKeys } from "@/lib/task-refresh";
import { taskIsPending, taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import {
  detectionControlBlocked,
  detectionControlLabels,
  type DetectionControlAction,
} from "../lib/account-control";

type ControlSelection = { accountID: string; action: DetectionControlAction };
const ControlContext = createContext<{
  pending: boolean;
  select: (value: ControlSelection) => void;
} | null>(null);

export function useDetectionAccountControl() {
  return useContext(ControlContext);
}

export function DetectionAccountControlProvider(props: { children: ReactNode }): ReactElement {
  const client = useQueryClient();
  const [selection, setSelection] = useState<ControlSelection | null>(null);
  const [running, setRunning] = useState<(ControlSelection & { taskID: string }) | null>(null);
  const notified = useRef<string | null>(null);
  const current = useQuery({
    queryKey: ["account-detail", selection?.accountID],
    queryFn: () => api.account(selection!.accountID),
    enabled: selection !== null,
    staleTime: 0,
    retry: false,
  });
  const task = useQuery({
    queryKey: ["account-scheduling", running?.accountID, running?.taskID],
    queryFn: () => api.task(running!.taskID),
    enabled: running !== null,
    refetchInterval: taskPollInterval,
  });
  const control = useMutation({
    mutationFn: (value: ControlSelection) => api.setAccountControl(value.accountID, value.action),
    onSuccess: (created, value) => {
      client.setQueryData(["account-scheduling", value.accountID, created.id], created);
      setRunning({ ...value, taskID: created.id });
      setSelection(null);
      void client.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (error) => notifyOperationError(error, "账号处置启动失败"),
  });
  useEffect(() => {
    if (!running || !task.data || !taskStopsPolling(task.data) || notified.current === task.data.id)
      return;
    notified.current = task.data.id;
    void Promise.all([
      client.invalidateQueries({ queryKey: ["account-detail", running.accountID] }),
      ...terminalRefreshKeys("account-scheduling", task.data).map((queryKey) =>
        client.invalidateQueries({ queryKey }),
      ),
    ]);
    notifyTaskResult(task.data, detectionControlLabels[running.action]);
  }, [client, running, task.data]);
  const pending = control.isPending || taskIsPending(running?.taskID ?? null, task);
  const label = selection ? detectionControlLabels[selection.action] : "账号处置";
  const account = current.data;
  const matching = account && selection && account.id === selection.accountID;
  const blocked = matching
    ? detectionControlBlocked(account, selection.action)
    : "账号状态尚未确认";
  const disabled =
    pending || current.isFetching || current.isError || !matching || Boolean(blocked);
  const identity = matching
    ? `${account.name}（ID：${account.id}）`
    : `账号 ID：${selection?.accountID ?? ""}`;
  const description =
    selection?.action === "fuse"
      ? `手动熔断“${identity}”后，该账号会立即停止接收流量，并持续保持熔断状态，直到手动解除。`
      : `确认恢复“${identity}”的调度？恢复后该账号可重新接收流量，后续仍受调度策略约束。`;
  return (
    <ControlContext.Provider value={{ pending, select: setSelection }}>
      {props.children}
      {task.isError ? (
        <QueryErrorToast error={task.error} fallback="账号处置任务读取失败，正在重试" />
      ) : null}
      <Dialog
        open={selection !== null}
        onOpenChange={(open) => {
          if (!open && !control.isPending) setSelection(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{label}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            {current.isFetching ? <ContentLoading label="正在读取账号状态" /> : null}
            {current.isError ? (
              <>
                <QueryErrorToast error={current.error} fallback="账号状态读取失败，请重试" />
                <ContentRetry onRetry={() => void current.refetch()} pending={current.isFetching} />
              </>
            ) : null}
            {!current.isFetching && !current.isError && blocked ? (
              <p className="text-sm text-muted-foreground">{blocked}</p>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={control.isPending}
              onClick={() => setSelection(null)}
            >
              取消
            </Button>
            <Button
              variant={selection?.action === "fuse" ? "destructive" : "default"}
              disabled={disabled}
              onClick={() => {
                if (selection && !disabled) control.mutate(selection);
              }}
            >
              {control.isPending ? "正在提交…" : `确认${label}`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </ControlContext.Provider>
  );
}
