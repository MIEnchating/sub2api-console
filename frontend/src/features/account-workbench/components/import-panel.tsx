import { useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api } from "@/api";
import { FileUpload } from "@/components/file-upload";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ArrowRight, FileText } from "lucide-react";
import { ContentLoading } from "@/components/content-loading";
import { notifyOperationError } from "@/lib/operation-feedback";
import { importModelKey, runKeys } from "../constants";
import { importDefaults, importSchema, type ImportValues } from "../lib/import-schema";
import { matchesImportPreviewSource } from "../lib/import-preview-source";
import type { ImportModelSettings, WorkbenchPreview } from "../types";
import { ImportPreview } from "./import-preview";
import { ImportSettings } from "./import-settings";

export function ImportPanel(props: { onStarted: () => void }): ReactElement {
  const client = useQueryClient();
  const form = useForm<ImportValues>({
    resolver: zodResolver(importSchema),
    defaultValues: importDefaults,
  });
  const values = form.watch();
  const [fileName, setFileName] = useState("");
  const [reading, setReading] = useState(false);
  const [modelReady, setModelReady] = useState(false);
  const [proxyReady, setProxyReady] = useState(true);
  const [preview, setPreview] = useState<{ value: WorkbenchPreview; source: ImportValues } | null>(
    null,
  );
  const parse = useMutation({
    gcTime: 0,
    mutationFn: (input: ImportValues) =>
      api.workbenchPreview({
        ...input,
        action: "import",
        promote: true,
        proxy_enabled: false,
        proxy_url: "",
      }),
    onSuccess: (value, input) => setPreview({ value, source: { ...input } }),
    onError: (error) => notifyOperationError(error, "账号解析失败"),
  });
  const start = useMutation({
    mutationFn: (value: WorkbenchPreview) =>
      api.startWorkbenchRun({ id: value.id, revision: value.revision }),
    onSuccess: () => {
      form.reset({
        ...importDefaults,
        model:
          client.getQueryData<ImportModelSettings>(importModelKey)?.model ?? importDefaults.model,
      });
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
    validated.success && preview && matchesImportPreviewSource(validated.data, preview.source)
      ? preview.value
      : null;
  return (
    <section aria-label="导入账号" className="flex h-full min-h-0 min-w-0 flex-col gap-4">
      <form
        onSubmit={form.handleSubmit((input) => {
          if (modelReady && proxyReady) parse.mutate(input);
        })}
        className="flex min-h-0 min-w-0 flex-1 flex-col"
      >
        <div className="grid min-h-0 min-w-0 flex-1 auto-rows-max content-start gap-4 overflow-y-auto overscroll-contain pb-4 lg:auto-rows-auto lg:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)] lg:content-stretch">
          <section
            aria-labelledby="workbench-input-heading"
            className="grid min-w-0 gap-4 rounded-lg border bg-card p-4 shadow-xs sm:p-5 lg:flex lg:min-h-80 lg:flex-col"
          >
            <div className="flex items-center gap-3 border-b pb-4">
              <span
                aria-hidden="true"
                className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-semibold text-primary"
              >
                1
              </span>
              <div className="min-w-0 space-y-1">
                <h2 id="workbench-input-heading" className="text-sm font-semibold">
                  <label htmlFor="workbench-input">账号资料</label>
                </h2>
                <p className="text-xs text-muted-foreground">粘贴内容或上传文件，自动识别格式</p>
              </div>
              <FileText
                aria-hidden="true"
                className="ml-auto size-5 shrink-0 text-muted-foreground"
              />
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
              className="h-48 min-h-48 shrink-0 field-sizing-fixed resize-none bg-muted/20 p-3 font-mono text-sm leading-7 lg:h-auto lg:max-h-none lg:flex-1 lg:shrink"
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
            busy={busy}
            onProxyReadyChange={setProxyReady}
            onModelReadyChange={setModelReady}
          />
        </div>
        <div className="flex shrink-0 items-center justify-between gap-3 rounded-lg border bg-card p-3 shadow-xs sm:px-4">
          <div className="flex min-w-0 items-center gap-3 text-xs text-muted-foreground">
            <span
              aria-hidden="true"
              className="hidden size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-semibold sm:flex"
            >
              3
            </span>
            {parse.isPending ? (
              <ContentLoading compact label="正在解析账号资料" />
            ) : (
              <span className="leading-5">先预览账号，确认后开始处理</span>
            )}
          </div>
          <Button type="submit" disabled={busy || !proxyReady || !modelReady}>
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
