import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api } from "@/api";
import { FileUpload } from "@/components/file-upload";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ArrowRight } from "lucide-react";
import { ContentLoading } from "@/components/content-loading";
import { notifyOperationError } from "@/lib/operation-feedback";
import { runKeys, templateKeys } from "../constants";
import { importDefaults, importSchema, type ImportValues } from "../lib/import-schema";
import type { WorkbenchPreview } from "../types";
import { ImportPreview } from "./import-preview";
import { ImportSettings } from "./import-settings";

export function ImportPanel(props: { onStarted: () => void }): ReactElement {
  const client = useQueryClient();
  const templates = useQuery({ queryKey: templateKeys.library, queryFn: api.workbenchTemplates });
  const form = useForm<ImportValues>({
    resolver: zodResolver(importSchema),
    defaultValues: importDefaults,
  });
  const values = form.watch();
  const [fileName, setFileName] = useState("");
  const [reading, setReading] = useState(false);
  const [preview, setPreview] = useState<{ value: WorkbenchPreview; source: string } | null>(null);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: (input: ImportValues) =>
      api.workbenchPreview({
        ...input,
        template_id: input.action === "export" ? "" : input.template_id,
        proxy_url: input.proxy_enabled ? input.proxy_url : "",
      }),
    onSuccess: (value, input) => setPreview({ value, source: JSON.stringify(input) }),
    onError: (error) => notifyOperationError(error, "账号解析失败"),
  });
  const start = useMutation({
    mutationFn: (value: WorkbenchPreview) =>
      api.startWorkbenchRun({ id: value.id, revision: value.revision }),
    onSuccess: () => {
      form.reset(importDefaults);
      setPreview(null);
      parse.reset();
      void client.invalidateQueries({ queryKey: runKeys.list });
      props.onStarted();
    },
    onError: (error) => notifyOperationError(error, "账号处理启动失败"),
  });
  const loadFile = async (file: File): Promise<void> => {
    if (file.size > 4 * 1024 * 1024) {
      toast.error("文件不能超过 4 MB，请分批处理");
      return;
    }
    setReading(true);
    try {
      const text = await file.text();
      form.setValue("content", text, { shouldValidate: true });
      setFileName(file.name);
      setPreview(null);
    } catch (error) {
      notifyOperationError(error, "文件读取失败，请重新选择");
    } finally {
      setReading(false);
    }
  };
  const busy = reading || parse.isPending || start.isPending;
  const validated = importSchema.safeParse(values);
  const activePreview =
    validated.success && preview?.source === JSON.stringify(validated.data) ? preview.value : null;
  return (
    <section aria-label="导入账号" className="flex h-full min-h-0 min-w-0 flex-col gap-4">
      <form
        onSubmit={form.handleSubmit((input) => parse.mutate(input))}
        className="flex min-h-0 min-w-0 flex-1 flex-col"
      >
        <div className="grid min-h-0 min-w-0 flex-1 auto-rows-max content-start gap-4 overflow-y-auto overscroll-contain pb-4 lg:auto-rows-auto lg:grid-cols-[minmax(0,1fr)_22rem] lg:content-stretch">
          <section
            aria-labelledby="workbench-input-heading"
            className="grid min-w-0 gap-3 rounded-xl border bg-card p-4 lg:flex lg:min-h-80 lg:flex-col"
          >
            <div className="flex items-center gap-2.5">
              <span
                aria-hidden="true"
                className="flex size-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-xs font-semibold text-primary"
              >
                1
              </span>
              <h2 id="workbench-input-heading" className="text-sm font-semibold">
                <label htmlFor="workbench-input">账号资料</label>
              </h2>
              <span className="ml-auto text-xs text-muted-foreground">自动识别格式</span>
            </div>
            <p id="workbench-input-hint" className="text-xs leading-5 text-muted-foreground">
              粘贴 JSON、RT 或邮箱登录资料，支持多行混合输入；也可上传文件。
            </p>
            <Textarea
              id="workbench-input"
              disabled={busy}
              aria-invalid={!!form.formState.errors.content}
              aria-describedby="workbench-input-hint"
              placeholder={'邮箱----密码----2FA 密钥\n\nrt_…\n\n{ "accounts": […] }'}
              className="h-40 min-h-40 shrink-0 field-sizing-fixed resize-none bg-muted/20 font-mono text-sm leading-6 lg:h-auto lg:min-h-48 lg:max-h-none lg:flex-1 lg:shrink"
              {...form.register("content")}
            />
            {form.formState.errors.content && (
              <p role="alert" className="text-sm text-destructive">
                {form.formState.errors.content.message}
              </p>
            )}
            <div className="shrink-0">
              <FileUpload
                label="上传账号资料"
                accept=".json,.txt,application/json,text/plain"
                description="拖入 JSON / TXT 文件，最大 4 MB。"
                fileName={fileName}
                busy={reading}
                disabled={busy}
                onSelect={(file) => void loadFile(file)}
              />
            </div>
          </section>
          <ImportSettings
            form={form}
            templates={templates.data}
            loading={templates.isPending}
            busy={busy}
          />
        </div>
        <div className="flex shrink-0 items-center justify-between gap-3 rounded-xl border bg-card p-3">
          <div className="min-w-0 text-xs text-muted-foreground">
            {parse.isPending ? (
              <ContentLoading compact label="正在解析账号资料" />
            ) : (
              "先预览账号，确认后开始处理"
            )}
          </div>
          <Button type="submit" disabled={busy}>
            解析并预览
            <ArrowRight aria-hidden="true" />
          </Button>
        </div>
      </form>
      {activePreview && (
        <ImportPreview
          preview={activePreview}
          pending={start.isPending}
          onClose={() => setPreview(null)}
          onConfirm={() => start.mutate(activePreview)}
        />
      )}
    </section>
  );
}
