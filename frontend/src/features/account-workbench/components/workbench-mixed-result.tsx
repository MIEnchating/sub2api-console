import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation } from "@tanstack/react-query";
import { api, type Task, type WorkbenchPreview } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";
import { WorkbenchPreviewPanel } from "./workbench-preview";

export function WorkbenchMixedResult(props: {
  id: string;
  exportOnly: boolean;
  disabled: boolean;
  onTask: (task: Task) => void;
  onBusy: (busy: boolean) => void;
  autoLoad?: boolean;
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
    mutationFn: async () => {
      const current = generation.current;
      const result = await api.previewWorkbenchRunResult(props.id);
      if (!mounted.current || current !== generation.current) {
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
    onError: (error) => notifyOperationError(error, "账号批次结果预览失败，请重试"),
  });
  const create = useMutation({
    mutationFn: (id: string) => api.importWorkbenchPreview(id),
    onSuccess: (task) => {
      activeID.current = null;
      setPreview(null);
      if (mounted.current) props.onTask(task);
    },
    onError: (error) => {
      clear();
      notifyOperationError(error, "账号批次导入任务创建失败，请重新预览");
    },
  });
  const busy = parse.isPending || create.isPending || converting;
  const writing = create.isPending || converting;
  useEffect(() => {
    props.onBusy(writing);
  }, [writing, props.onBusy]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      clear();
    };
  }, [clear]);
  const load = parse.mutate;
  useEffect(() => {
    if (props.autoLoad) load();
  }, [props.autoLoad, load]);
  return (
    <div className="grid min-w-0 gap-3">
      {!preview && !parse.isPending && (
        <div>
          <Button
            disabled={props.disabled || busy}
            onClick={() => {
              clear();
              parse.mutate();
            }}
          >
            {props.exportOnly ? "预览可导出账号" : "预览可导入账号"}
          </Button>
        </div>
      )}
      {parse.isPending && <ContentLoading label="正在生成账号批次结果预览" />}
      {create.isPending && <TaskStartupState message="正在创建账号批次导入任务" />}
      {preview && (
        <WorkbenchPreviewPanel
          preview={preview}
          pending={props.disabled || parse.isPending || create.isPending}
          onConversionPendingChange={setConverting}
          onConverted={(task) => {
            activeID.current = null;
            setPreview(null);
            props.onTask(task);
          }}
          onDiscard={clear}
          onConfirm={() => {
            if (!props.exportOnly) create.mutate(preview.id);
          }}
        />
      )}
    </div>
  );
}
