import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { FileOutput } from "lucide-react";
import {
  api,
  type Task,
  type WorkbenchLoginProfile,
  type WorkbenchProfileExportPreview,
  type WorkbenchSourceProfile,
  type WorkbenchScope,
} from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";

export function WorkbenchProfileExport(props: {
  profiles: Array<WorkbenchLoginProfile | WorkbenchSourceProfile>;
  scope?: WorkbenchScope;
  disabled?: boolean;
  onCreated: (task: Task) => void;
}): ReactElement {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<WorkbenchProfileExportPreview | null>(null);
  const [expired, setExpired] = useState(false);
  const previewID = useRef<string | null>(null);
  const generation = useRef(0);
  const close = (): void => {
    generation.current += 1;
    const id = previewID.current;
    previewID.current = null;
    if (id) void api.discardWorkbenchExportPreview(id).catch(() => undefined);
    setPreview(null);
    setOpen(false);
  };
  const read = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const current = generation.current;
      const preview =
        props.scope === "local-export"
          ? api.previewWorkbenchSourceProfileExport
          : api.previewWorkbenchProfileExport;
      const value = await preview(
        props.profiles.map((profile) => ({ id: profile.id, revision: profile.revision })),
      );
      if (current !== generation.current) {
        await api.discardWorkbenchExportPreview(value.id);
        return null;
      }
      return value;
    },
    onSuccess: (value) => {
      if (!value) return;
      previewID.current = value.id;
      setPreview(value);
    },
    onError: (error) => {
      notifyOperationError(error, "登录资料导出预览失败，请刷新资料后重试");
      void client.invalidateQueries({
        queryKey:
          props.scope === "local-export"
            ? ["account-workbench", "source-profiles"]
            : workbenchKeys.profiles,
      });
    },
  });
  const create = useMutation({
    mutationFn:
      props.scope === "local-export"
        ? api.createWorkbenchSourceProfileExport
        : api.createWorkbenchProfileExport,
    onSuccess: (task) => {
      previewID.current = null;
      close();
      props.onCreated(task);
      toast.success("登录资料私有导出任务已创建");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => {
      close();
      notifyOperationError(error, "登录资料导出未启动，请重新预览后确认");
      void client.invalidateQueries({
        queryKey:
          props.scope === "local-export"
            ? ["account-workbench", "source-profiles"]
            : workbenchKeys.profiles,
      });
    },
  });
  useEffect(
    () => () => {
      generation.current += 1;
      const id = previewID.current;
      previewID.current = null;
      if (id) void api.discardWorkbenchExportPreview(id).catch(() => undefined);
    },
    [],
  );
  useEffect(() => {
    if (!preview) return;
    const remaining = Date.parse(preview.expires_at) - Date.now();
    setExpired(!Number.isFinite(remaining) || remaining <= 0);
    if (!Number.isFinite(remaining) || remaining <= 0) return;
    const timer = setTimeout(() => setExpired(true), remaining);
    return () => clearTimeout(timer);
  }, [preview]);
  return (
    <>
      <Button
        variant="outline"
        disabled={
          props.disabled ||
          !props.profiles.length ||
          props.profiles.length > 500 ||
          read.isPending ||
          create.isPending
        }
        onClick={() => {
          setOpen(true);
          setPreview(null);
          read.mutate();
        }}
      >
        <FileOutput aria-hidden="true" />
        私有导出资料（{props.profiles.length}）
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!value && !create.isPending) close();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认导出登录资料</DialogTitle>
            <DialogDescription>
              将下列账号保存的密码、TOTP、收码和代理配置写入服务器私有文件，保留 24
              小时。界面仅展示文件编号。
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="grid gap-3">
            {read.isPending && <ContentLoading label="正在读取资料导出范围" />}
            {read.isError && !preview && (
              <ContentRetry pending={read.isPending} onRetry={() => read.mutate()} />
            )}
            {preview && (
              <>
                <p className="text-sm wrap-anywhere">
                  {preview.scope === "local-export"
                    ? "范围：本地登录资料"
                    : `管理目标：${preview.target}`}
                </p>
                <ul className="space-y-2 text-sm" aria-label="资料导出范围">
                  {preview.items.map((profile) => (
                    <li key={profile.id} className="wrap-anywhere">
                      {profile.email}
                      <span className="block text-xs text-muted-foreground">
                        {"account_id" in profile ? `账号 ID：${profile.account_id}；` : ""}资料 ID：
                        {profile.id}；版本：
                        {profile.revision}
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            )}
            {expired && preview && (
              <p role="status" className="text-sm">
                资料预览已过期，请关闭后重新预览。
              </p>
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" disabled={create.isPending} onClick={close}>
              返回
            </Button>
            <Button
              disabled={read.isPending || !preview?.items.length || expired || create.isPending}
              onClick={() => {
                if (preview && !expired) create.mutate(preview.id);
              }}
            >
              {create.isPending ? "正在创建导出任务…" : "确认生成资料文件"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
