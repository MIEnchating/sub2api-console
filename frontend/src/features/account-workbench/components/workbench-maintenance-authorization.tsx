import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type WorkbenchMaintenance } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";
import { WorkbenchOAuthBatchBrowser } from "./workbench-oauth-batch-browser";
import { WorkbenchOAuthBatchRows } from "./workbench-oauth-batch-rows";
import { WorkbenchTask } from "./workbench-task";
import { workbenchKeys } from "../constants";

export function WorkbenchMaintenanceAuthorization(props: {
  config: WorkbenchMaintenance;
}): ReactElement {
  const [confirm, setConfirm] = useState(false);
  const [cancel, setCancel] = useState<string | null>(null);
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["account-workbench", "maintenance-authorization"],
    queryFn: api.workbenchMaintenanceAuthorization,
    refetchInterval: 3000,
  });
  const batchID = query.data?.current_reauthorization_id;
  const batch = useQuery({
    queryKey: workbenchKeys.batch(batchID ?? null),
    queryFn: (context) => api.workbenchOAuthBatch(batchID!, context.signal),
    enabled: !!batchID,
    gcTime: 0,
    retry: false,
    refetchInterval: (state) => (state.state.error ? false : 1000),
  });
  const importID = query.data?.current_import_task_id;
  const task = useQuery({
    queryKey: workbenchKeys.task(importID ?? ""),
    queryFn: () => api.task(importID!),
    enabled: !!importID,
  });
  const attach = useMutation({
    mutationFn: () => api.attachWorkbenchMaintenance(props.config.revision),
    onSuccess: () => {
      setConfirm(false);
      void query.refetch();
      void client.invalidateQueries({ queryKey: workbenchKeys.maintenance });
    },
    onError: (error) => notifyOperationError(error, "自动重新授权未连接，请刷新维护设置后重试"),
  });
  const stop = useMutation({
    mutationFn: (id: string) => api.cancelWorkbenchOAuthBatch(id),
    onSuccess: () => {
      setCancel(null);
      void query.refetch();
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "取消自动授权失败，请刷新当前任务后重试"),
  });
  if (query.isPending) return <ContentLoading label="正在读取自动授权状态" />;
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  return (
    <section aria-label="维护账号重新授权" className="min-w-0 space-y-3">
      <p role="status" className="text-sm">
        {query.data.attached
          ? "当前登录会话已连接自动重新授权"
          : "自动重新授权等待连接当前登录会话"}
      </p>
      {!query.data.attached ? (
        <Button variant="outline" disabled={attach.isPending} onClick={() => setConfirm(true)}>
          连接本次登录会话
        </Button>
      ) : null}
      {batch.isPending && batchID ? <ContentLoading label="正在读取当前授权队列" /> : null}
      {batch.isError ? (
        <ContentRetry pending={batch.isFetching} onRetry={() => void batch.refetch()} />
      ) : null}
      {batch.data ? (
        <>
          <p className="text-sm wrap-anywhere">{batch.data.message}</p>
          <WorkbenchOAuthBatchRows items={batch.data.items} />
          {batch.data.current_oauth_id ? (
            <WorkbenchOAuthBatchBrowser
              key={batch.data.current_oauth_id}
              id={batch.data.current_oauth_id}
              disabled={batch.isError || stop.isPending}
            />
          ) : null}
          {batch.data.status === "queued" || batch.data.status === "running" ? (
            <Button
              variant="outline"
              disabled={stop.isPending}
              onClick={() => setCancel(batch.data.id)}
            >
              取消本轮重新授权
            </Button>
          ) : null}
        </>
      ) : null}
      {task.data ? <WorkbenchTask key={task.data.id} task={task.data} /> : null}
      <ConfirmActionDialog
        open={confirm}
        title="确认连接自动重新授权"
        description={`范围：${props.config.group_ids.length ? `分组 ID ${props.config.group_ids.join("、")}` : "全部 OpenAI OAuth 账号"}。使用已保存的登录资料重新授权需要修复的账号，成功后更新原账号凭据。不会自动购买短信。退出登录或重启服务后需重新连接。`}
        confirmLabel="确认连接"
        pending={attach.isPending}
        onOpenChange={setConfirm}
        onConfirm={() => attach.mutate()}
      />
      <ConfirmActionDialog
        open={cancel !== null}
        title="确认取消本轮重新授权"
        description="停止当前队列的后续授权和导入。已提交的账号变更会保留，请核对维护任务结果。"
        confirmLabel="取消本轮授权"
        pending={stop.isPending}
        onOpenChange={(open) => {
          if (!open) setCancel(null);
        }}
        onConfirm={() => {
          if (cancel) stop.mutate(cancel);
        }}
      />
    </section>
  );
}
