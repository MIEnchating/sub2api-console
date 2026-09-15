import { useEffect, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Play, Trash2 } from "lucide-react";
import { api, type WorkbenchQueueRecovery, type WorkbenchScope } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
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
import { queueItemStatusLabels, workbenchKeys } from "../constants";

export function WorkbenchQueueRecovery(props: {
  scope?: WorkbenchScope;
  kind: WorkbenchQueueRecovery["kind"];
  disabled: boolean;
  onResume: (value: WorkbenchQueueRecovery) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" disabled={props.disabled} onClick={() => setOpen(true)}>
        <History aria-hidden="true" />
        恢复已保存批次
      </Button>
      {open && (
        <QueueDialog
          scope={props.scope}
          kind={props.kind}
          onClose={() => setOpen(false)}
          onResume={(value) => {
            setOpen(false);
            props.onResume(value);
          }}
        />
      )}
    </>
  );
}

function QueueDialog(props: {
  scope?: WorkbenchScope;
  kind: WorkbenchQueueRecovery["kind"];
  onClose: () => void;
  onResume: (value: WorkbenchQueueRecovery) => void;
}): ReactElement {
  const client = useQueryClient();
  const [now, setNow] = useState(Date.now);
  const [selection, setSelection] = useState<{
    queue: WorkbenchQueueRecovery;
    action: "resume" | "delete";
  } | null>(null);
  const query = useQuery({
    queryKey: [...workbenchKeys.queueRecoveries, props.scope ?? "managed"],
    queryFn: (context) => api.workbenchQueueRecoveries(context.signal, props.scope),
    gcTime: 0,
    retry: false,
  });
  const queues = query.data?.filter((queue) => queue.kind === props.kind);
  useEffect(() => {
    const expiries =
      query.data?.map((item) => Date.parse(item.expires_at)).filter((value) => value > now) ?? [];
    if (!expiries.length) return;
    const timer = setTimeout(() => setNow(Date.now()), Math.min(...expiries) - now);
    return () => clearTimeout(timer);
  }, [query.data, now]);
  const remove = useMutation({
    mutationFn: api.deleteWorkbenchQueueRecovery,
    onSuccess: () => {
      setSelection(null);
      void client.invalidateQueries({ queryKey: workbenchKeys.queueRecoveries });
    },
    onError: (error) => {
      notifyOperationError(error, "恢复资料删除失败，请刷新后重试");
      setSelection(null);
      void query.refetch();
    },
  });
  return (
    <>
      <Dialog
        open
        onOpenChange={(value) => {
          if (!value && !remove.isPending) props.onClose();
        }}
      >
        <DialogContent width="progress">
          <DialogHeader>
            <DialogTitle>恢复已保存批次</DialogTitle>
            <DialogDescription>
              继续未执行项目并复用已保存的成功结果。提交结果不明的项目须单独核对，原到期时间不延长。
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="grid gap-3">
            {query.isPending && <ContentLoading label="正在读取批量恢复资料" />}
            {query.isError && (
              <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
            )}
            {queues?.length === 0 && (
              <p className="text-sm text-muted-foreground">暂无可恢复批次</p>
            )}
            <ul className="divide-y" aria-label="批量恢复资料">
              {queues?.map((queue) => (
                <li key={queue.id} className="space-y-2 py-3 text-sm wrap-anywhere">
                  <p>来源任务：{queue.task_id}</p>
                  {queue.items && (
                    <>
                      <p>
                        待执行 {queue.pending} 项；已成功 {queue.succeeded} 项；待核对{" "}
                        {queue.review} 项
                      </p>
                      <details className="min-w-0">
                        <summary className="cursor-pointer">
                          账号范围（{queue.items.length} 项）
                        </summary>
                        <ul
                          aria-label="恢复账号范围"
                          className="mt-2 max-h-52 overflow-auto divide-y"
                        >
                          {queue.items.map((item) => (
                            <li key={item.index} className="grid min-w-0 gap-1 py-2">
                              <span>
                                第 {item.index + 1} 项：{item.email || "待识别账号"}
                              </span>
                              {item.workspace_id && (
                                <span className="text-xs text-muted-foreground">
                                  工作区：{item.workspace_id}
                                </span>
                              )}
                              <span className="text-xs">
                                {Object.hasOwn(queueItemStatusLabels, item.status)
                                  ? queueItemStatusLabels[item.status]
                                  : "待核对"}
                              </span>
                            </li>
                          ))}
                        </ul>
                      </details>
                    </>
                  )}
                  {queue.active && <p>原任务仍在当前服务中</p>}
                  <p className="text-xs text-muted-foreground">到期：{queue.expires_at}</p>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      disabled={
                        !queue.can_resume ||
                        query.isError ||
                        query.isFetching ||
                        remove.isPending ||
                        !(Date.parse(queue.expires_at) > now)
                      }
                      onClick={() => setSelection({ queue, action: "resume" })}
                    >
                      <Play aria-hidden="true" />
                      恢复本批
                    </Button>
                    <Button
                      variant="outline"
                      disabled={
                        queue.active ||
                        queue.status === "running" ||
                        remove.isPending ||
                        query.isFetching ||
                        query.isError
                      }
                      onClick={() => setSelection({ queue, action: "delete" })}
                    >
                      <Trash2 aria-hidden="true" />
                      删除恢复资料
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" disabled={remove.isPending} onClick={props.onClose}>
              返回
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmActionDialog
        open={selection !== null}
        title={selection?.action === "resume" ? "确认恢复批次" : "删除批量恢复资料"}
        description={
          selection?.action === "resume"
            ? `确认接回来源任务 ${selection.queue.task_id}。本批未执行的账号继续处理，已保存结果复用；原接码与手机号绑定仍按已确认配置执行。`
            : "永久删除服务器保存的本批输入与尚未使用的成功结果，之后无法恢复。"
        }
        confirmLabel={selection?.action === "resume" ? "确认继续本批" : "确认删除恢复资料"}
        pending={remove.isPending}
        confirmDisabled={
          query.isError ||
          query.isFetching ||
          (selection?.action === "resume" &&
            (!selection.queue.can_resume || !(Date.parse(selection.queue.expires_at) > now)))
        }
        onOpenChange={(value) => {
          if (!value) setSelection(null);
        }}
        onConfirm={() => {
          if (!selection || query.isError || query.isFetching) return;
          if (selection.action === "delete") remove.mutate(selection.queue);
          else if (
            selection.queue.can_resume &&
            Date.parse(selection.queue.expires_at) > Date.now()
          )
            props.onResume({ ...selection.queue, scope: props.scope ?? selection.queue.scope });
        }}
      />
    </>
  );
}
