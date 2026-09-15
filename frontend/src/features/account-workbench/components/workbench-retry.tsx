import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Task, type WorkbenchPreview } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import type { RetryValues } from "../lib/schemas";
import { WorkbenchPreviewPanel } from "./workbench-preview";
import { WorkbenchRetryOptions } from "./workbench-retry-options";

export function WorkbenchRetry(props: {
  task: Task;
  indexes: number[];
  onClose: () => void;
  onCreated: (task: Task) => void;
}): ReactElement {
  const client = useQueryClient();
  const [preview, setPreview] = useState<WorkbenchPreview | null>(null);
  const active = useRef<string | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const discard = useCallback((): void => {
    generation.current += 1;
    const id = active.current;
    active.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
  }, []);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async (values: RetryValues) => {
      const requestedGeneration = generation.current;
      const result = await api.workbenchRetryPreview({
        task_id: props.task.id,
        indexes: props.indexes,
        model: values.model,
        template_id: values.template_id || undefined,
      });
      if (!mounted.current || generation.current !== requestedGeneration) {
        await api.discardWorkbenchPreview(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (value) => {
      if (!value) return;
      active.current = value.id;
      setPreview(value);
    },
    onError: (error) => notifyOperationError(error, "重试范围读取失败，请核对原任务后重试"),
  });
  const create = useMutation({
    mutationFn: api.importWorkbenchPreview,
    onSuccess: (task) => {
      active.current = null;
      discard();
      toast.success("账号重新处理任务已创建");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      props.onCreated(task);
    },
    onError: (error) => notifyOperationError(error, "重新处理任务创建失败，请重新预览"),
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current += 1;
      const id = active.current;
      active.current = null;
      if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
    };
  }, []);
  return (
    <section aria-label="重新处理账号" className="grid min-w-0 gap-3 border-t pt-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-base font-medium">重新处理 {props.indexes.length} 项</h2>
        <Button variant="outline" disabled={create.isPending} onClick={props.onClose}>
          结束重试
        </Button>
      </div>
      <p className="text-sm wrap-anywhere">
        原任务 {props.task.id}；条目：{props.indexes.map((index) => index + 1).join("、")}
      </p>
      <WorkbenchRetryOptions
        disabled={parse.isPending || create.isPending}
        onChange={discard}
        onSubmit={(values) => {
          discard();
          parse.mutate(values);
        }}
      />
      {parse.isPending && <ContentLoading label="正在核对原任务和账号范围" />}
      {preview && (
        <WorkbenchPreviewPanel
          preview={preview}
          pending={create.isPending}
          onConfirm={() => create.mutate(preview.id)}
          onDiscard={discard}
        />
      )}
      {create.isPending && <TaskStartupState message="正在创建重新处理任务" />}
    </section>
  );
}
