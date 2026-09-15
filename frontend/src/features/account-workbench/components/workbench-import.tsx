import { useEffect, useRef, useState, type ChangeEvent, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
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
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { notifyOperationError } from "@/lib/operation-feedback";
import { maxInputBytes, workbenchKeys } from "../constants";
import { importSchema, type ImportValues } from "../lib/schemas";
import { usePreferredTemplate } from "../hooks/use-preferred-template";
import { WorkbenchPreviewPanel } from "./workbench-preview";
import { WorkbenchTask } from "./workbench-task";

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
  const [format, setFormat] = useState(local ? "json" : "tokens");
  const [preview, setPreview] = useState<WorkbenchPreview | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [reading, setReading] = useState(false);
  const [converting, setConverting] = useState(false);
  const activePreview = useRef<string | null>(null);
  const mounted = useRef(true);
  const fileInput = useRef<HTMLInputElement>(null);
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
    if (fileInput.current) fileInput.current.value = "";
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
  async function loadFile(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
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
      setFormat(
        content.trimStart().startsWith("{") || content.trimStart().startsWith("[")
          ? "json"
          : "tokens",
      );
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
        className="grid min-w-0 gap-4 rounded-lg border bg-card p-4"
        onChange={clearPreview}
        onSubmit={form.handleSubmit(() => {
          clearPreview();
          parse.mutate();
        })}
      >
        <div
          className="flex flex-col gap-3 sm:flex-row sm:items-end"
          role="group"
          aria-label="账号输入方式"
        >
          <FormField label="输入格式">
            <Select
              value={format}
              onValueChange={(value) => setFormat(value ?? "tokens")}
              disabled={busy}
            >
              <SelectTrigger aria-label="输入格式">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {!local && <SelectItem value="tokens">Refresh Token 文本</SelectItem>}
                <SelectItem value="json">账号 JSON</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <div className="min-w-0 flex-1">
            <label htmlFor="workbench-file" className="mb-1.5 block text-sm font-medium">
              从文件读取（最大 2 MB）
            </label>
            <Input
              ref={fileInput}
              id="workbench-file"
              type="file"
              accept=".json,.txt,.jsonl"
              disabled={busy}
              onChange={(event) => void loadFile(event)}
            />
          </div>
        </div>
        <FormField
          label="账号内容"
          htmlFor="workbench-content"
          error={form.formState.errors.content?.message}
        >
          {format === "json" ? (
            <JsonEditorField
              control={form.control}
              name="content"
              aria-label="账号内容"
              disabled={busy}
              onValueChange={clearPreview}
              className="h-64"
            />
          ) : (
            <Textarea
              id="workbench-content"
              aria-label="账号内容"
              aria-invalid={!!form.formState.errors.content}
              className="h-64 resize-y font-mono"
              placeholder="每行填写一个 rt_ 开头的 Refresh Token，也支持邮箱与 Token 组合。"
              autoComplete="off"
              spellCheck={false}
              disabled={busy}
              {...form.register("content")}
            />
          )}
        </FormField>
        {!local && (
          <div className="grid min-w-0 gap-4 sm:grid-cols-2">
            <FormField label="配置模板">
              <Controller
                control={form.control}
                name="template_id"
                render={({ field }) => (
                  <Select
                    value={field.value || "auto"}
                    disabled={busy}
                    onValueChange={(value) => {
                      clearPreview();
                      field.onChange(value === "auto" ? "" : value);
                    }}
                  >
                    <SelectTrigger aria-label="配置模板" className="min-w-0">
                      <SelectValue>
                        <span className="min-w-0 truncate">
                          {templates.data?.find((item) => item.id === field.value)?.name ||
                            "自动匹配模板"}
                        </span>
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto">自动匹配模板</SelectItem>
                      {templates.data?.map((item) => (
                        <SelectItem key={item.id} value={item.id}>
                          {item.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </FormField>
            {props.output !== "export" ? (
              <FormField
                label="检测模型"
                htmlFor="workbench-import-model"
                error={form.formState.errors.model?.message}
              >
                <Input
                  id="workbench-import-model"
                  disabled={busy || !form.watch("check_after_import")}
                  aria-invalid={!!form.formState.errors.model}
                  placeholder="填写已支持的模型名称"
                  {...form.register("model")}
                />
              </FormField>
            ) : null}
          </div>
        )}
        {props.output !== "export" ? (
          <label className="flex items-center gap-2 text-sm">
            <Controller
              control={form.control}
              name="check_after_import"
              render={({ field }) => (
                <Checkbox
                  checked={field.value}
                  disabled={busy}
                  onCheckedChange={(value) => {
                    clearPreview();
                    field.onChange(value);
                  }}
                />
              )}
            />
            导入后检测（会产生模型调用用量）
          </label>
        ) : null}
        {!local && (
          <p className="text-sm text-muted-foreground">
            {props.output === "export"
              ? "输入账号凭据并套用模板，确认后生成服务器私有 JSON 文件。输入凭据仅用于本次转换。"
              : "先预览识别结果和配置，再确认导入至系统设置中的 Sub2API。输入凭据仅用于本次处理。"}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
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
            }}
          >
            清空输入
          </Button>
        </div>
        {reading && <ContentLoading label="正在读取账号文件" compact />}
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
