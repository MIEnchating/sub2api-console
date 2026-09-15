import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { FormProvider, useForm } from "react-hook-form";
import { toast } from "sonner";
import { FileJson, Search, Trash2, Upload } from "lucide-react";
import { api, type WorkbenchRunInput, type WorkbenchScope } from "@/api";
import { FormField } from "@/components/form-field";
import { FileUpload } from "@/components/file-upload";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { notifyOperationError } from "@/lib/operation-feedback";
import { maxInputBytes, workbenchKeys } from "../constants";
import { mixedRunDefaults, mixedRunSchema, type MixedRunValues } from "../lib/mixed-run-schema";
import { usePreferredTemplate } from "../hooks/use-preferred-template";
import { WorkbenchMixedOptions } from "./workbench-mixed-options";
import { WorkbenchImportSkeleton } from "./workbench-page-skeletons";

export function WorkbenchMixedForm(props: {
  scope?: WorkbenchScope;
  onSubmit: (input: WorkbenchRunInput) => void;
  onProceed?: (input: WorkbenchRunInput) => void;
}): ReactElement {
  const local = props.scope === "local-export";
  const client = useQueryClient();
  const [reading, setReading] = useState(false);
  const [fileName, setFileName] = useState("");
  const mounted = useRef(true);
  const form = useForm<MixedRunValues>({
    resolver: zodResolver(mixedRunSchema),
    defaultValues: { ...mixedRunDefaults, export_only: local },
  });
  const exportOnly = form.watch("export_only");
  const content = form.watch("content");
  const templates = useQuery({
    queryKey: workbenchKeys.templates,
    queryFn: api.workbenchTemplates,
    enabled: !local && !exportOnly,
  });
  usePreferredTemplate(
    local ? undefined : templates.data,
    (id) => form.setValue("template_id", id),
    true,
  );
  const preference = useMutation({
    mutationFn: async (id: string) => {
      const item = templates.data?.find((value) => (id ? value.id === id : value.preferred));
      if (item) await api.setPreferredWorkbenchTemplate(item.id, item.revision, !!id);
      return id;
    },
    onSuccess: (id) => {
      form.setValue("template_id", id);
      void client.invalidateQueries({ queryKey: workbenchKeys.templates });
    },
    onError: (error) => notifyOperationError(error, "模板选择失败，请刷新模板后重试"),
  });
  const busy = reading || preference.isPending;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      form.reset({
        ...mixedRunDefaults,
        export_only: local,
        template_id: form.getValues("template_id"),
      });
    };
  }, [form, local]);
  async function loadFile(file: File): Promise<void> {
    if (file.size > maxInputBytes) {
      toast.error("文件不能超过 2 MB，请分批运行");
      return;
    }
    setReading(true);
    try {
      const content = await file.text();
      if (mounted.current) {
        form.setValue("content", content, { shouldValidate: true });
        setFileName(file.name);
      }
    } catch (error) {
      notifyOperationError(error, "文件读取失败，请重新选择文件");
    } finally {
      if (mounted.current) setReading(false);
    }
  }
  if (!local && !exportOnly && templates.isPending) return <WorkbenchImportSkeleton />;
  return (
    <form
      aria-label="账号导入输入"
      className="@container/mixed grid min-w-0 gap-3"
      onSubmit={form.handleSubmit((values, event) => {
        const submitter = (event?.nativeEvent as SubmitEvent | undefined)?.submitter;
        const proceed = submitter?.getAttribute("value") === "run";
        const input: WorkbenchRunInput = {
          scope: values.export_only ? "local-export" : props.scope,
          recovery_enabled: false,
          content: values.content,
          template_id: local || values.export_only ? undefined : values.template_id || undefined,
          check_after_import: !local && !values.export_only,
          model: values.export_only ? "" : values.model || mixedRunDefaults.model,
          export_only: local || values.export_only,
          proxy_url: values.proxy_url || undefined,
        };
        form.reset({
          ...mixedRunDefaults,
          template_id: values.template_id,
          export_only: values.export_only,
        });
        setFileName("");
        if (proceed && props.onProceed) props.onProceed(input);
        else props.onSubmit(input);
      })}
    >
      <FormProvider {...form}>
        {!local && !exportOnly && !templates.data && (
          <ContentRetry pending={templates.isFetching} onRetry={() => void templates.refetch()} />
        )}
        <fieldset
          disabled={busy}
          aria-label="混合内容与选项"
          className="grid min-w-0 items-start gap-5 @3xl/mixed:grid-cols-[minmax(0,1fr)_20rem]"
        >
          <section aria-label="账号内容与文件" className="grid min-w-0 gap-3">
            <FormField label="从文件读取（最大 2 MB）" htmlFor="mixed-run-file">
              <FileUpload
                id="mixed-run-file"
                label="从文件读取（最大 2 MB）"
                description="TXT / JSON / JSONL，最大 2 MB"
                accept=".txt,.json,.jsonl"
                fileName={fileName}
                busy={reading}
                onSelect={(file) => void loadFile(file)}
              />
            </FormField>
            <FormField
              label="账号内容"
              htmlFor="mixed-run-content"
              error={form.formState.errors.content?.message}
              reserveErrorSpace={false}
            >
              <Textarea
                id="mixed-run-content"
                className="h-64 resize-y font-mono"
                placeholder={'邮箱----密码----2FA 密钥\n\nrt_...\n\n{ "accounts": [...] }'}
                autoComplete="off"
                spellCheck={false}
                aria-invalid={!!form.formState.errors.content}
                {...form.register("content")}
              />
            </FormField>
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm text-muted-foreground">
                {content.trim() ? `${content.trim().split(/\r?\n/).length} 行内容` : "尚未添加账号"}
              </span>
              <Button
                type="button"
                variant="ghost"
                disabled={busy || !content}
                onClick={() => {
                  form.setValue("content", "", { shouldValidate: false });
                  setFileName("");
                }}
              >
                <Trash2 aria-hidden="true" />
                清空输入
              </Button>
            </div>
          </section>
          <WorkbenchMixedOptions
            local={local}
            templates={templates.data ?? []}
            disabled={busy}
            onTemplateChange={(id) => preference.mutate(id)}
          >
            <div className="grid gap-2">
              <Button
                type="submit"
                name="action"
                value="run"
                disabled={busy || !content.trim() || (!local && !exportOnly && templates.isError)}
              >
                {exportOnly ? <FileJson aria-hidden="true" /> : <Upload aria-hidden="true" />}
                {exportOnly ? "生成 JSON" : "导入并检测"}
              </Button>
              <Button
                type="submit"
                name="action"
                value="preview"
                variant="outline"
                disabled={busy || !content.trim() || (!local && !exportOnly && templates.isError)}
              >
                <Search aria-hidden="true" />
                解析并预览
              </Button>
            </div>
          </WorkbenchMixedOptions>
        </fieldset>
      </FormProvider>
    </form>
  );
}
