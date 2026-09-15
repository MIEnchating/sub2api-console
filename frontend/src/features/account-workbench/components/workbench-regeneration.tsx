import { useEffect, useId, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Task, type WorkbenchRegenerationInput } from "@/api";
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
import { workbenchKeys } from "../constants";

export function WorkbenchRegeneration(props: {
  source: WorkbenchRegenerationInput;
  onClose: () => void;
  onCreated: (task: Task) => void;
}): ReactElement {
  const client = useQueryClient();
  const mounted = useRef(true);
  const instance = useId();
  const id = useRef<string | null>(null);
  const [expired, setExpired] = useState(false);
  const parse = useQuery({
    queryKey: [...workbenchKeys.exports, "regeneration-preview", instance],
    gcTime: 0,
    staleTime: Infinity,
    retry: false,
    queryFn: async () => {
      const value = await api.previewWorkbenchRegeneration(props.source);
      if (!mounted.current) {
        await api.discardWorkbenchExportPreview(value.id);
        return null;
      }
      id.current = value.id;
      return value;
    },
  });
  const preview = parse.data;
  const create = useMutation({
    mutationFn: api.regenerateWorkbenchExport,
    onSuccess: (task) => {
      id.current = null;
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      props.onCreated(task);
      props.onClose();
    },
    onError: (error) => {
      props.onClose();
      notifyOperationError(error, "重新生成任务未启动，请重新预览");
    },
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const current = id.current;
      id.current = null;
      if (current) void api.discardWorkbenchExportPreview(current).catch(() => undefined);
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
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !create.isPending) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>确认重新生成授权文件</DialogTitle>
          <DialogDescription>
            使用现有 Refresh Token
            刷新授权，可能使旧令牌失效。每个成功账号立即保存为服务器私有文件，保留 24
            小时；线上账号凭据不会自动更新。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-3">
          {parse.isPending && <ContentLoading label="正在核对重新生成范围" />}
          {parse.isError && !preview && (
            <ContentRetry pending={parse.isFetching} onRetry={() => void parse.refetch()} />
          )}
          {preview && (
            <>
              <p className="text-sm wrap-anywhere">
                {preview.scope === "local-export"
                  ? "范围：本地私有文件"
                  : `管理目标：${preview.target}`}
              </p>
              <ul aria-label="重新生成授权范围" className="space-y-3 text-sm">
                {preview.items.map((item) => (
                  <li key={item.index} className="wrap-anywhere">
                    第 {item.index + 1} 项：{item.name || item.email}
                    <span className="block text-xs text-muted-foreground">
                      {item.account_id && `账号 ID：${item.account_id}；`}用户：{item.user_id}
                      ；工作区：{item.workspace_id}
                    </span>
                  </li>
                ))}
              </ul>
            </>
          )}
          {expired && <p role="status">预览已过期，请关闭后重新选择来源。</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={create.isPending} onClick={props.onClose}>
            返回
          </Button>
          <Button
            disabled={!preview?.items.length || parse.isPending || expired || create.isPending}
            onClick={() => {
              if (preview && !expired) create.mutate(preview.id);
            }}
          >
            {create.isPending ? "正在创建任务…" : "确认刷新并生成文件"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
