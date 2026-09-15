import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type Task, type WorkbenchPreview, type WorkbenchScope } from "@/api";
import { FormField } from "@/components/form-field";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { WorkbenchImportSkeleton } from "./workbench-page-skeletons";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { FileUpload } from "@/components/file-upload";
import { notifyOperationError } from "@/lib/operation-feedback";
import { cn } from "@/lib/utils";
import { maxInputBytes, workbenchKeys } from "../constants";
import { importSchema, type ImportValues } from "../lib/schemas";
import { usePreferredTemplate } from "../hooks/use-preferred-template";
import { WorkbenchPreviewPanel } from "./workbench-preview";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchImportOptions } from "./workbench-import-options";

const defaults: ImportValues = {
  content: "",
  template_id: "",
  check_after_import: false,
  model: "",
};

export function WorkbenchImport(
  props: { output?: "export"; scope?: WorkbenchScope } = {},
): ReactElement {
  const client = useQueryClient();
  const local = props.scope === "local-export";
  const templates = useQuery({
    queryKey: workbenchKeys.templates,
    queryFn: api.workbenchTemplates,
    enabled: !local,
  });
  const [preview, setPreview] = useState<WorkbenchPreview | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [reading, setReading] = useState(false);
  const [fileName, setFileName] = useState("");
  const [converting, setConverting] = useState(false);
  const activePreview = useRef<string | null>(null);
  const mounted = useRef(true);
  const form = useForm<ImportValues>({
    resolver: zodResolver(importSchema),
    defaultValues: defaults,
  });
  usePreferredTemplate(local ? undefined : templates.data, (id) =>
    form.setValue("template_id", id),
  );
  function clearPreview(): void {
    const id = activePreview.current;
    activePreview.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
  }
  const parse = useMutation({
    mutationFn: () =>
      api.workbenchPreview({
        ...form.getValues(),
        scope: props.scope,
        export_only: props.output === "export" || undefined,
        template_id: local ? undefined : form.getValues("template_id") || undefined,
      }),
    gcTime: 0,
    onSuccess: (value) => {
      if (!mounted.current) {
        void api.discardWorkbenchPreview(value.id).catch(() => undefined);
        return;
      }
      activePreview.current = value.id;
      setPreview(value);
    },
    onError: (error) => notifyOperationError(error, "账号解析失败，请检查输入后重试"),
  });
  const create = useMutation({
    mutationFn: (id: string) => api.importWorkbenchPreview(id),
    onSuccess: (value) => {
      acceptTask(value);
      toast.success("导入任务已创建");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "导入任务创建失败，请重新预览后重试"),
  });
  function acceptTask(value: Task): void {
    activePreview.current = null;
    setPreview(null);
    form.reset({ ...defaults, template_id: form.getValues("template_id") });
    parse.reset();
    setFileName("");
    setTask(value);
  }
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const id = activePreview.current;
      activePreview.current = null;
      if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
      form.reset(defaults);
    };
  }, [form]);
  const busy = parse.isPending || create.isPending || reading || converting;
  async function loadFile(file: File): Promise<void> {
    if (file.size > maxInputBytes) {
      toast.error("文件不能超过 2 MB，请分批导入");
      return;
    }
    clearPreview();
    setReading(true);
    try {
      const content = await file.text();
      if (!mounted.current) return;
      form.setValue("content", content, { shouldValidate: true });
      setFileName(file.name);
    } catch (error) {
      notifyOperationError(error, "文件读取失败，请重新选择文件");
    } finally {
      if (mounted.current) setReading(false);
    }
  }
  if (!local && templates.isPending) return <WorkbenchImportSkeleton />;
  if (!local && !templates.data)
    return <ContentRetry pending={templates.isFetching} onRetry={() => void templates.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      <form
        aria-label="账号导入输入"
        className="@container/import grid min-w-0 gap-3"
        onChange={clearPreview}
        onSubmit={form.handleSubmit(() => {
          clearPreview();
          parse.mutate();
        })}
      >
        <div
          role="group"
          aria-label="导入内容与选项"
          className={cn(
            "grid min-w-0 items-start gap-5",
            !local && "@3xl/import:grid-cols-[minmax(0,1fr)_20rem]",
          )}
        >
          <section aria-label="账号内容与文件" className="grid min-w-0 gap-3">
            <div className="grid min-w-0 gap-3" role="group" aria-label="账号输入方式">
              <div className="min-w-0 flex-1">
                <label htmlFor="workbench-file" className="mb-1.5 block text-sm font-medium">
                  从文件读取（最大 2 MB）
                </label>
                <FileUpload
                  id="workbench-file"
                  label="从文件读取（最大 2 MB）"
                  description="TXT / JSON / JSONL，最大 2 MB"
                  accept=".json,.txt,.jsonl"
                  fileName={fileName}
                  busy={reading}
                  disabled={busy}
                  onSelect={(file) => void loadFile(file)}
                />
              </div>
            </div>
            <FormField
              label="账号内容"
              htmlFor="workbench-content"
              error={form.formState.errors.content?.message}
              reserveErrorSpace={false}
            >
              <JsonEditorField
                control={form.control}
                name="content"
                id="workbench-content"
                aria-label="账号内容"
                language="auto"
                placeholder="账号 JSON 或 rt_ 刷新令牌"
                disabled={busy}
                onValueChange={clearPreview}
                className="h-64"
              />
            </FormField>
          </section>
          {!local && (
            <WorkbenchImportOptions
              form={form}
              templates={templates.data ?? []}
              exportOnly={props.output === "export"}
              disabled={busy}
              onChange={clearPreview}
            />
          )}
        </div>
        <div role="group" aria-label="账号输入操作" className="flex flex-wrap items-center gap-2">
          <Button type="submit" disabled={busy}>
            解析并预览
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={() => {
              clearPreview();
              form.reset(defaults);
              setFileName("");
            }}
          >
            清空输入
          </Button>
        </div>
        {parse.isPending && <ContentLoading label="正在解析账号并读取影响范围" compact />}
        {create.isPending && <TaskStartupState message="正在创建账号导入任务" />}
      </form>
      {preview && (
        <WorkbenchPreviewPanel
          preview={preview}
          pending={create.isPending}
          onConverted={acceptTask}
          onConversionPendingChange={setConverting}
          onConfirm={() => create.mutate(preview.id)}
          onDiscard={clearPreview}
        />
      )}
      {task && <WorkbenchTask task={task} />}
    </div>
  );
}
