import { useEffect, useId, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Task, type WorkbenchLoginProfile } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import { cleanupKindLabels, taskStatusLabels, workbenchKeys } from "../constants";

export function WorkbenchCleanup(props: {
  profiles: WorkbenchLoginProfile[];
  onClose: () => void;
  onCreated: (task: Task) => void;
}): ReactElement {
  const client = useQueryClient();
  const instance = useId();
  const mounted = useRef(true);
  const previewID = useRef<string | null>(null);
  const [expired, setExpired] = useState(false);
  const query = useQuery({
    queryKey: [...workbenchKeys.profiles, "cleanup-preview", instance],
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    queryFn: async () => {
      const value = await api.previewWorkbenchCleanup(
        props.profiles.map((profile) => ({ id: profile.id, revision: profile.revision })),
      );
      if (!mounted.current) {
        if (value.id) await api.discardWorkbenchCleanupPreview(value.id);
        return null;
      }
      previewID.current = value.id || null;
      return value;
    },
  });
  const preview = query.data;
  const create = useMutation({
    mutationFn: api.cleanupWorkbenchAccounts,
    onSuccess: (task) => {
      previewID.current = null;
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      props.onCreated(task);
      props.onClose();
    },
    onError: (error) => {
      notifyOperationError(error, "清理任务未启动，请刷新范围后重新确认");
      props.onClose();
    },
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const id = previewID.current;
      previewID.current = null;
      if (id) void api.discardWorkbenchCleanupPreview(id).catch(() => undefined);
    };
  }, []);
  useEffect(() => {
    if (!preview) return;
    const remaining = Date.parse(preview.expires_at) - Date.now();
    setExpired(!Number.isFinite(remaining) || remaining <= 0);
    if (!Number.isFinite(remaining) || remaining <= 0) return;
    const timer = setTimeout(() => setExpired(true), remaining);
    return () => clearTimeout(timer);
  }, [preview]);
  const deletions = preview?.items.filter((item) => item.action === "delete") ?? [];
  return (
    <Dialog
      open
      onOpenChange={(value) => {
        if (!value && !create.isPending) props.onClose();
      }}
    >
      <DialogContent width="progress">
        <DialogHeader>
          <DialogTitle>确认清理账号关联资料</DialogTitle>
          <DialogDescription>
            永久删除列为“删除”的服务器资料，线上账号与独立审计保留。包含其他账号或无法确认归属的记录和文件会保留；清理后无法通过这些资料恢复授权。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-3">
          {query.isPending && <ContentLoading label="正在核对关联资料清理范围" />}
          {query.isError && (
            <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
          )}
          {preview && (
            <>
              <p className="text-sm wrap-anywhere">管理目标：{preview.target}</p>
              <ul aria-label="清理所选账号" className="space-y-1 text-sm">
                {preview.profiles.map((profile) => (
                  <li key={profile.id} className="wrap-anywhere">
                    {profile.email}（账号 ID {profile.account_id}，资料版本 {profile.revision}）
                  </li>
                ))}
              </ul>
              {preview.blocked && (
                <section
                  aria-label="阻止清理的活动任务"
                  className="grid gap-2 border-y py-3 text-sm"
                >
                  <p>存在活动任务，请等待结束后重新预览。</p>
                  <ul className="space-y-1">
                    {preview.blockers.map((task) => (
                      <li key={task.task_id} className="wrap-anywhere">
                        {task.message} · {taskStatusLabels[task.status]}
                        <span className="block text-xs text-muted-foreground">
                          任务 ID：{task.task_id}
                        </span>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
              <p className="text-sm">
                删除 {deletions.length} 项，保留 {preview.items.length - deletions.length} 项
              </p>
              <ul aria-label="关联资料清理明细" className="divide-y">
                {preview.items.map((item) => (
                  <li
                    key={`${item.kind}:${item.id}`}
                    className="space-y-1 py-2 text-sm wrap-anywhere"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant={item.action === "delete" ? "destructive" : "secondary"}>
                        {item.action === "delete" ? "删除" : "保留"}
                      </Badge>
                      <span>
                        {cleanupKindLabels[item.kind]} · {item.count} 项
                      </span>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      ID：{item.id}；账号：{item.account_ids.join("、") || "未确认"}
                    </p>
                    <p>{item.reason}</p>
                  </li>
                ))}
              </ul>
            </>
          )}
          {expired && <p role="status">清理预览已过期，请关闭后重新核对。</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={create.isPending} onClick={props.onClose}>
            返回
          </Button>
          <Button
            variant="destructive"
            disabled={
              !preview?.id ||
              preview.blocked ||
              !deletions.length ||
              expired ||
              query.isError ||
              query.isFetching ||
              create.isPending
            }
            onClick={() => {
              if (preview && !preview.blocked && !expired) create.mutate(preview.id);
            }}
          >
            {create.isPending ? "正在创建清理任务…" : "确认永久删除所列资料"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
