import { useEffect, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Play, ShieldCheck, Trash2 } from "lucide-react";
import {
  api,
  type WorkbenchOAuthCheckpoint,
  type WorkbenchScope,
  type WorkbenchOAuthSession,
} from "@/api";
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
import { checkpointStatusLabels, workbenchKeys } from "../constants";
import { WorkbenchSourceSecurityBatch } from "./workbench-source-security-batch";

export function WorkbenchOAuthCheckpoints(props: {
  scope?: WorkbenchScope;
  disabled: boolean;
  onRestore: (value: WorkbenchOAuthCheckpoint) => void;
  onSecurity?: (value: WorkbenchOAuthCheckpoint) => void;
  onOAuth?: (session: WorkbenchOAuthSession) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" disabled={props.disabled} onClick={() => setOpen(true)}>
        <History aria-hidden="true" />
        已暂停的授权
      </Button>
      {open && (
        <CheckpointDialog
          scope={props.scope}
          onOAuth={
            props.onOAuth
              ? (session) => {
                  setOpen(false);
                  props.onOAuth?.(session);
                }
              : undefined
          }
          onClose={() => setOpen(false)}
          onRestore={(value) => {
            setOpen(false);
            props.onRestore(value);
          }}
          onSecurity={
            props.onSecurity
              ? (value) => {
                  setOpen(false);
                  props.onSecurity?.(value);
                }
              : undefined
          }
        />
      )}
    </>
  );
}

function CheckpointDialog(props: {
  scope?: WorkbenchScope;
  onClose: () => void;
  onRestore: (value: WorkbenchOAuthCheckpoint) => void;
  onSecurity?: (value: WorkbenchOAuthCheckpoint) => void;
  onOAuth?: (session: WorkbenchOAuthSession) => void;
}): ReactElement {
  const client = useQueryClient();
  const [securityOpen, setSecurityOpen] = useState(false);
  const [selected, setSelected] = useState<{
    value: WorkbenchOAuthCheckpoint;
    action: "restore" | "delete";
  } | null>(null);
  const [now, setNow] = useState(Date.now);
  const query = useQuery({
    queryKey: [...workbenchKeys.checkpoints, props.scope ?? "managed"],
    queryFn: (context) => api.workbenchOAuthCheckpoints(context.signal, props.scope),
    gcTime: 0,
    retry: false,
  });
  useEffect(() => {
    const expiries =
      query.data?.map((item) => Date.parse(item.expires_at)).filter((value) => value > now) ?? [];
    if (!expiries.length) return;
    const timer = setTimeout(() => setNow(Date.now()), Math.min(...expiries) - now);
    return () => clearTimeout(timer);
  }, [query.data, now]);
  const remove = useMutation({
    mutationFn: api.deleteWorkbenchOAuthCheckpoint,
    onSuccess: () => {
      setSelected(null);
      void client.invalidateQueries({ queryKey: workbenchKeys.checkpoints });
    },
    onError: (error) => {
      setSelected(null);
      notifyOperationError(error, "检查点删除失败，请刷新后重试");
      void query.refetch();
    },
  });
  const restorable =
    selected?.action === "restore" &&
    selected.value.can_restore &&
    Date.parse(selected.value.expires_at) > now;
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
            <DialogTitle>已暂停的授权</DialogTitle>
            <DialogDescription>
              恢复后需人工继续登录。有效期以原授权到期时间为准；已使用的检查点无法再次恢复。
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="grid gap-3">
            {query.isPending && <ContentLoading label="正在读取授权检查点" />}
            {query.isError && (
              <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
            )}
            {query.data?.length === 0 && (
              <p className="text-sm text-muted-foreground">暂无已暂停的授权</p>
            )}
            <ul aria-label="授权检查点" className="divide-y">
              {query.data?.map((item) => (
                <li key={item.id} className="space-y-2 py-3 text-sm wrap-anywhere">
                  <p>
                    {item.active ? "原授权仍在进行" : checkpointStatusLabels[item.status]} ·
                    来源任务：{item.source_task_id}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    到期：{item.expires_at}；检查点：{item.id}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      disabled={
                        query.isError ||
                        query.isFetching ||
                        remove.isPending ||
                        !item.can_restore ||
                        !(Date.parse(item.expires_at) > now)
                      }
                      onClick={() => setSelected({ value: item, action: "restore" })}
                    >
                      <Play aria-hidden="true" />
                      恢复授权
                    </Button>
                    <Button
                      variant="outline"
                      disabled={
                        query.isFetching || remove.isPending || item.active || !!item.parent_task_id
                      }
                      onClick={() => setSelected({ value: item, action: "delete" })}
                    >
                      <Trash2 aria-hidden="true" />
                      删除检查点
                    </Button>
                    {props.onSecurity ? (
                      <Button
                        variant="outline"
                        disabled={
                          query.isError ||
                          query.isFetching ||
                          remove.isPending ||
                          item.active ||
                          !!item.parent_task_id ||
                          !item.can_restore ||
                          !(Date.parse(item.expires_at) > now)
                        }
                        onClick={() =>
                          props.onSecurity?.({ ...item, scope: props.scope ?? item.scope })
                        }
                      >
                        <ShieldCheck aria-hidden="true" />
                        转入安全设置
                      </Button>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={
                query.isError ||
                query.isFetching ||
                remove.isPending ||
                !query.data?.some(
                  (item) =>
                    !item.active &&
                    !item.parent_task_id &&
                    item.can_restore &&
                    Date.parse(item.expires_at) > now,
                )
              }
              onClick={() => setSecurityOpen(true)}
            >
              <ShieldCheck aria-hidden="true" />
              批量设置检查点账号安全
            </Button>
            <Button variant="outline" disabled={remove.isPending} onClick={props.onClose}>
              返回
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {securityOpen ? (
        <WorkbenchSourceSecurityBatch
          scope={props.scope}
          onOAuth={props.onOAuth}
          sources={(query.data ?? [])
            .filter(
              (item) =>
                !item.active &&
                !item.parent_task_id &&
                item.can_restore &&
                Date.parse(item.expires_at) > now,
            )
            .map((item) => ({
              source: {
                checkpoint_id: item.id,
                revision: item.revision,
                checkpoint_revision: item.checkpoint_revision,
              },
              label: `来源任务：${item.source_task_id}`,
            }))}
          onClose={() => {
            setSecurityOpen(false);
            void query.refetch();
          }}
        />
      ) : null}
      <ConfirmActionDialog
        open={selected !== null}
        title={selected?.action === "restore" ? "确认恢复授权" : "删除授权检查点"}
        description={
          selected?.action === "restore"
            ? `恢复来源任务 ${selected.value.source_task_id} 的授权。邮箱、短信与密码自动填写不会恢复，请人工继续登录。`
            : "永久删除此检查点中的私有浏览器状态，之后无法从此处继续授权。"
        }
        confirmLabel={selected?.action === "restore" ? "确认人工恢复" : "确认删除检查点"}
        pending={remove.isPending}
        confirmDisabled={
          query.isError || query.isFetching || (selected?.action === "restore" && !restorable)
        }
        onOpenChange={(value) => {
          if (!value) setSelected(null);
        }}
        onConfirm={() => {
          if (!selected || query.isError || query.isFetching) return;
          if (selected.action === "delete") remove.mutate(selected.value);
          else if (restorable && Date.parse(selected.value.expires_at) > Date.now())
            props.onRestore({ ...selected.value, scope: props.scope ?? selected.value.scope });
          else {
            setSelected(null);
            void query.refetch();
          }
        }}
      />
    </>
  );
}
