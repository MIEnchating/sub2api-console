import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Task, type WorkbenchExportPreview } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { WorkbenchExportsSkeleton } from "./workbench-page-skeletons";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import { WorkbenchExportSelection } from "./workbench-export-selection";
import { WorkbenchExportPreviewPanel } from "./workbench-export-preview";
import { WorkbenchExportArtifacts } from "./workbench-export-artifacts";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchRegeneration } from "./workbench-regeneration";

export function WorkbenchExports(props: { sourceTaskId?: string } = {}): ReactElement {
  const client = useQueryClient();
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: api.accounts,
    enabled: !props.sourceTaskId,
  });
  const [preview, setPreview] = useState<WorkbenchExportPreview | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [regenerating, setRegenerating] = useState<string[] | null>(null);
  const activePreview = useRef<string | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const discard = useCallback((): void => {
    generation.current += 1;
    const id = activePreview.current;
    activePreview.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchExportPreview(id).catch(() => undefined);
  }, []);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async (ids: string[]) => {
      const requestedGeneration = generation.current;
      const value = props.sourceTaskId
        ? await api.workbenchBatchExportPreview(props.sourceTaskId)
        : await api.workbenchExportPreview(ids);
      if (!mounted.current || generation.current !== requestedGeneration) {
        await api.discardWorkbenchExportPreview(value.id);
        return null;
      }
      return value;
    },
    onSuccess: (value) => {
      if (!value) return;
      activePreview.current = value.id;
      setPreview(value);
    },
    onError: (error) => notifyOperationError(error, "导出预览失败，请刷新账号后重试"),
  });
  const create = useMutation({
    mutationFn: api.createWorkbenchExport,
    onSuccess: (value) => {
      activePreview.current = null;
      discard();
      setTask(value);
      toast.success("私有导出任务已创建");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "导出任务创建失败，请重新预览后重试"),
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current += 1;
      const id = activePreview.current;
      activePreview.current = null;
      if (id) void api.discardWorkbenchExportPreview(id).catch(() => undefined);
    };
  }, []);
  if (!props.sourceTaskId && accounts.isPending) return <WorkbenchExportsSkeleton />;
  if (!props.sourceTaskId && !accounts.data)
    return <ContentRetry pending={accounts.isFetching} onRetry={() => void accounts.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      {props.sourceTaskId && !preview && !task && (
        <div>
          <Button
            variant="outline"
            disabled={parse.isPending || create.isPending}
            onClick={() => {
              discard();
              parse.mutate([]);
            }}
          >
            导出本批结果
          </Button>
        </div>
      )}
      {!props.sourceTaskId && (
        <WorkbenchExportSelection
          accounts={accounts.data ?? []}
          disabled={parse.isPending || create.isPending}
          onChange={discard}
          onRegenerate={(ids) => {
            discard();
            setRegenerating(ids);
          }}
          onSubmit={(ids) => {
            discard();
            parse.mutate(ids);
          }}
        />
      )}
      {parse.isPending && (
        <div className="grid gap-2">
          <ContentLoading label="正在生成导出预览" />
          <div>
            <Button
              variant="outline"
              onClick={() => {
                discard();
                parse.reset();
              }}
            >
              取消预览
            </Button>
          </div>
        </div>
      )}
      {preview && (
        <WorkbenchExportPreviewPanel
          preview={preview}
          pending={create.isPending}
          onConfirm={() => create.mutate(preview.id)}
          onDiscard={discard}
        />
      )}
      {create.isPending && <TaskStartupState message="正在创建私有导出任务" />}
      {task && <WorkbenchTask task={task} />}
      {regenerating && (
        <WorkbenchRegeneration
          source={{ account_ids: regenerating }}
          onClose={() => setRegenerating(null)}
          onCreated={setTask}
        />
      )}
      {!props.sourceTaskId && <WorkbenchExportArtifacts />}
    </div>
  );
}
