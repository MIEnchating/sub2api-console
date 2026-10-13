import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";
import { rememberedAccountKey, workbenchKeys } from "../constants";
import type { RememberedAccount, WorkbenchAccount } from "../types";

export type AccountActionTarget =
  | { kind: "platform"; account: WorkbenchAccount; schedulable?: boolean }
  | { kind: "remembered"; account: RememberedAccount };

export function AccountActionDialog(props: {
  target: AccountActionTarget;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const target = props.target;
  const remembered = target.kind === "remembered" ? target.account : null;
  const accountID = remembered?.account_id ?? target.account.id;
  const schedulable = target.kind === "platform" ? target.schedulable : undefined;
  const scheduling = schedulable !== undefined;
  const [linked, setLinked] = useState(false);
  const remote = !remembered || linked;
  const preview = useQuery({
    queryKey: ["account-workbench", "action-preview", accountID],
    queryFn: ({ signal }) => api.workbenchAccountActionPreview(accountID, signal),
    enabled: remote,
    staleTime: 0,
    gcTime: 0,
  });
  const refresh = (): void => {
    void client.invalidateQueries({ queryKey: workbenchKeys.accounts });
    void client.invalidateQueries({ queryKey: rememberedAccountKey });
    void client.invalidateQueries({ queryKey: ["accounts"] });
    void client.invalidateQueries({ queryKey: ["account-workbench", "account-groups"] });
  };
  const mutation = useMutation({
    mutationFn: async () => {
      if (remembered && !linked) {
        await api.deleteWorkbenchRememberedAccount(remembered.id, remembered.revision);
        return null;
      }
      if (!preview.data) throw new Error("请先读取账号影响范围");
      const deleteRemembered = !scheduling && (remembered !== null || linked);
      return api.startWorkbenchAccountAction(accountID, {
        action: scheduling ? "scheduling" : "delete",
        schedulable,
        account_version: preview.data.account_version,
        target_version: preview.data.target_version,
        delete_remembered: deleteRemembered,
        remembered_revision: deleteRemembered
          ? (remembered?.revision ?? preview.data.remembered?.revision ?? 0)
          : 0,
      });
    },
    onSuccess: (task) => {
      if (!task) {
        refresh();
        props.onClose();
      }
    },
    onError: (error) => notifyOperationError(error, scheduling ? "调度操作失败" : "账号删除失败"),
  });
  const taskQuery = useQuery({
    queryKey: ["account-workbench", "account-action-task", mutation.data?.id],
    queryFn: ({ signal }) => api.task(mutation.data!.id, signal),
    enabled: !!mutation.data,
    refetchInterval: (query) =>
      !query.state.data || ["queued", "running"].includes(query.state.data.status) ? 1000 : false,
  });
  const task = taskQuery.data ?? mutation.data;
  const running = mutation.isPending || (!!task && ["queued", "running"].includes(task.status));
  const notified = useRef<string | null>(null);
  useEffect(() => {
    if (!task || running || notified.current === task.id) return;
    notified.current = task.id;
    void client.invalidateQueries({ queryKey: workbenchKeys.accounts });
    void client.invalidateQueries({ queryKey: rememberedAccountKey });
    void client.invalidateQueries({ queryKey: ["accounts"] });
    void client.invalidateQueries({ queryKey: ["account-workbench", "account-groups"] });
    if (task.status === "succeeded") {
      toast.success(task.message || "账号操作完成");
      props.onClose();
    } else notifyOperationError(task.message, "账号操作未完成，请刷新列表核对");
  }, [client, task, running, props]);
  let title = remembered ? "删除已记住账号" : "删除平台账号";
  let confirm = "确认删除";
  if (scheduling) {
    title = schedulable ? "开启调度" : "关闭调度";
    confirm = schedulable ? "确认开启" : "确认关闭";
  }
  const ready = !remote || (!!preview.data && !preview.isFetching && !preview.isError);
  let description = remembered
    ? "删除已保存的登录资料及生成凭据，后续需重新导入才能使用。"
    : "从平台删除该账号，删除后无法撤销。";
  if (scheduling)
    description = schedulable
      ? "开启后账号可以接收调度流量，仍受平台状态与调度策略约束。"
      : "关闭后账号停止接收调度流量，账号配置与已记住资料保留。";
  let deletionScope = remembered ? "平台账号保留。" : "已记住账号及本地登录资料保留。";
  if (linked) deletionScope = "平台账号删除确认后，再清理本地登录资料及生成凭据。";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !running) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid gap-4">
          <p className="text-sm wrap-anywhere">
            {target.account.email || target.account.name}（#{accountID}）
          </p>
          <p className="text-sm">{description}</p>
          {!scheduling && (remembered || preview.data?.remembered) && (
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={linked}
                disabled={running}
                onCheckedChange={(value) => setLinked(value === true)}
              />
              {remembered ? "同时删除平台账号" : "同时删除已记住账号"}
            </label>
          )}
          {!scheduling && <p className="text-sm text-muted-foreground">{deletionScope}</p>}
          {remote && preview.isPending && <ContentLoading label="正在核对账号影响范围" />}
          {remote && preview.isError && (
            <ContentRetry pending={preview.isFetching} onRetry={() => void preview.refetch()} />
          )}
          {remote && preview.data && !preview.data.platform_present && (
            <p className="text-sm text-muted-foreground">平台账号已不存在，仅清理所选本地资料。</p>
          )}
          {running && <TaskStartupState message="正在执行账号操作…" />}
          {taskQuery.isError && (
            <ContentRetry pending={taskQuery.isFetching} onRetry={() => void taskQuery.refetch()} />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={running} onClick={props.onClose}>
            返回
          </Button>
          <Button
            variant={scheduling ? "default" : "destructive"}
            disabled={!ready || running || !!task}
            onClick={() => mutation.mutate()}
          >
            {confirm}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
