import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { useQuery } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";

import { api, type AccountDeletePreview, type Task } from "@/api";
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
import { taskStopsPolling } from "@/lib/task-state";

export const accountDeleteActionLabel = "删除账号";

export function AccountDeletePreviewDetails(props: { preview: AccountDeletePreview }) {
  return (
    <div className="grid gap-4">
      <div className="bg-destructive/10 text-destructive rounded-md px-3 py-2.5 text-sm">
        {props.preview.binding
          ? "将删除管理平台账号、该账号绑定的上游 Key，以及 Console 中对应的绑定和调度记录。"
          : "该账号没有可确认的上游 Key 绑定；将删除管理平台账号、确认其不存在并清理 Console 本地记录；不会删除任何上游 Key。"}
      </div>
      <div className="divide-y rounded-md border text-sm">
        <div className="grid grid-cols-[7rem_minmax(0,1fr)] gap-3 px-3 py-2.5">
          <span className="text-muted-foreground">管理平台账号</span>
          <strong className="min-w-0 break-words font-medium">
            {props.preview.account_name}（ID {props.preview.account_id}）
          </strong>
        </div>
        <div className="grid grid-cols-[7rem_minmax(0,1fr)] gap-3 px-3 py-2.5">
          <span className="text-muted-foreground">管理目标</span>
          <span className="min-w-0 break-all font-mono text-xs">
            {props.preview.management_base_url}
          </span>
        </div>
        {props.preview.binding ? (
          <>
            <div className="grid grid-cols-[7rem_minmax(0,1fr)] gap-3 px-3 py-2.5">
              <span className="text-muted-foreground">上游地址</span>
              <span className="min-w-0 break-all font-mono text-xs">
                {props.preview.binding.upstream_host}
              </span>
            </div>
            <div className="grid grid-cols-[7rem_minmax(0,1fr)] gap-3 px-3 py-2.5">
              <span className="text-muted-foreground">稳定上游身份</span>
              <span className="min-w-0 break-all font-mono text-xs">
                {props.preview.binding.upstream_id}
              </span>
            </div>
            <div className="grid grid-cols-[7rem_minmax(0,1fr)] gap-3 px-3 py-2.5">
              <span className="text-muted-foreground">上游 Key</span>
              <span className="min-w-0 break-words">
                {props.preview.binding.upstream_key_name || "未命名 Key"}（ID{" "}
                <span className="font-mono text-xs">{props.preview.binding.upstream_key_id}</span>）
              </span>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

export function AccountDeleteDialog(props: {
  accountId: string;
  open: boolean;
  pending: boolean;
  activeAction: string | null;
  task: Task | undefined;
  taskError: unknown;
  onOpenChange: (open: boolean) => void;
  onConfirm: (preview: AccountDeletePreview) => void;
}) {
  const deleting = props.activeAction === accountDeleteActionLabel;
  const preview = useQuery({
    queryKey: ["account-delete-preview", props.accountId],
    queryFn: () => api.accountDeletePreview(props.accountId),
    enabled: props.open && !deleting,
    retry: false,
  });

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent width="medium" showCloseButton={!props.pending}>
        <DialogHeader>
          <DialogTitle>
            {preview.data?.binding === null ? "删除管理平台账号" : "删除账号及上游 Key"}
          </DialogTitle>
          <DialogDescription>
            {preview.data?.binding === null
              ? "确认后会删除管理平台账号和 Console 本地记录，此操作不可撤销。"
              : "确认后会按稳定 ID 同时删除两端数据，此操作不可撤销。"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          {preview.isLoading ? <ContentLoading label="正在读取账号删除范围" /> : null}
          {preview.error ? (
            <>
              <QueryErrorToast error={preview.error} fallback="账号删除范围读取失败" />
              <ContentRetry onRetry={() => void preview.refetch()} pending={preview.isFetching} />
            </>
          ) : null}
          {preview.data && !deleting ? (
            <div className="grid gap-4">
              <AccountDeletePreviewDetails preview={preview.data} />
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => props.onOpenChange(false)}>
                  取消
                </Button>
                <Button
                  variant="destructive"
                  disabled={props.pending || preview.isFetching || preview.isError}
                  onClick={() => props.onConfirm(preview.data)}
                >
                  <Trash2 aria-hidden="true" />
                  确认删除
                </Button>
              </div>
            </div>
          ) : null}
          {deleting && props.task && !taskStopsPolling(props.task) ? (
            <TaskProgressState
              message={props.task.message}
              progress={props.task.progress}
              taskId={props.task.id}
            />
          ) : null}
          {deleting && !props.task && !props.taskError ? (
            <TaskStartupState message="正在创建账号删除任务" />
          ) : null}
          {deleting && props.taskError ? (
            <QueryErrorToast error={props.taskError} fallback="账号删除任务状态读取失败" />
          ) : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
