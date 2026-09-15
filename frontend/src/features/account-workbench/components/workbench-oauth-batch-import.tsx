import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation } from "@tanstack/react-query";
import { api, type Task, type WorkbenchPreview, type WorkbenchScope } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";
import type { OAuthPreviewValues } from "../lib/schemas";
import { WorkbenchOAuthOptions } from "./workbench-oauth-options";
import { WorkbenchPreviewPanel } from "./workbench-preview";

export function WorkbenchOAuthBatchImport(props: {
  scope?: WorkbenchScope;
  id: string;
  disabled: boolean;
  onImported: (task: Task) => void;
  onConverted?: (task: Task) => void;
  onPendingChange?: (pending: boolean) => void;
}): ReactElement {
  const [preview, setPreview] = useState<WorkbenchPreview | null>(null);
  const [converting, setConverting] = useState(false);
  const activeID = useRef<string | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const clear = useCallback((): void => {
    generation.current += 1;
    const id = activeID.current;
    activeID.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
  }, []);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async (input: OAuthPreviewValues) => {
      const current = generation.current;
      const result = await api.previewWorkbenchOAuthBatchImport(props.id, {
        ...input,
        scope: props.scope,
        export_only: props.scope === "local-export" || undefined,
        template_id: input.template_id || undefined,
      });
      if (!mounted.current || generation.current !== current) {
        await api.discardWorkbenchPreview(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (value) => {
      if (value) {
        activeID.current = value.id;
        setPreview(value);
      }
    },
    onError: (error) => notifyOperationError(error, "批量授权账号导入预览失败，请重试"),
  });
  const create = useMutation({
    mutationFn: (id: string) => api.importWorkbenchPreview(id),
    onSuccess: (task) => {
      activeID.current = null;
      setPreview(null);
      props.onImported(task);
    },
    onError: (error) => notifyOperationError(error, "批量授权账号导入任务创建失败，请重新预览"),
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      clear();
    };
  }, [clear]);
  const busy = props.disabled || parse.isPending || create.isPending || converting;
  const writing = create.isPending || converting;
  useEffect(() => {
    props.onPendingChange?.(writing);
    return () => props.onPendingChange?.(false);
  }, [writing, props.onPendingChange]);
  return (
    <div className="min-w-0 space-y-4">
      <WorkbenchOAuthOptions
        scope={props.scope}
        disabled={busy}
        onChange={clear}
        onSubmit={(values) => {
          clear();
          parse.mutate(values);
        }}
      />
      {parse.isPending && <ContentLoading label="正在生成批量授权账号导入预览" />}
      {create.isPending && <TaskStartupState message="正在创建批量授权账号导入任务" />}
      {preview && (
        <WorkbenchPreviewPanel
          preview={preview}
          pending={props.disabled || parse.isPending || create.isPending}
          onConversionPendingChange={setConverting}
          onConverted={(value) => {
            activeID.current = null;
            setPreview(null);
            if (props.onConverted) props.onConverted(value);
            else props.onImported(value);
          }}
          onDiscard={clear}
          onConfirm={() => create.mutate(preview.id)}
        />
      )}
    </div>
  );
}
