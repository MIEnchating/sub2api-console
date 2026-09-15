import { useEffect, useRef, useState, type ChangeEvent, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, FormProvider, useForm } from "react-hook-form";
import { toast } from "sonner";
import { api, type WorkbenchRunInput, type WorkbenchScope } from "@/api";
import { FormField } from "@/components/form-field";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { notifyOperationError } from "@/lib/operation-feedback";
import { maxInputBytes, workbenchKeys } from "../constants";
import { mixedRunDefaults, mixedRunSchema, type MixedRunValues } from "../lib/mixed-run-schema";
import { oauthSMSInput } from "../lib/oauth-sms-schema";
import { usePreferredTemplate } from "../hooks/use-preferred-template";
import { WorkbenchOAuthSMSFields } from "./workbench-oauth-sms";
import { WorkbenchImportSkeleton } from "./workbench-page-skeletons";

export function WorkbenchMixedForm(props: {
  scope?: WorkbenchScope;
  onSubmit: (input: WorkbenchRunInput) => void;
}): ReactElement {
  const local = props.scope === "local-export";
  const templates = useQuery({
    queryKey: workbenchKeys.templates,
    queryFn: api.workbenchTemplates,
    enabled: !local,
  });
  const [reading, setReading] = useState(false);
  const mounted = useRef(true);
  const form = useForm<MixedRunValues>({
    resolver: zodResolver(mixedRunSchema),
    defaultValues: { ...mixedRunDefaults, export_only: local },
  });
  const exportOnly = form.watch("export_only");
  usePreferredTemplate(local ? undefined : templates.data, (id) =>
    form.setValue("template_id", id),
  );
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      form.reset(mixedRunDefaults);
    };
  }, [form]);
  async function loadFile(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (file.size > maxInputBytes) {
      toast.error("文件不能超过 2 MB，请分批运行");
      return;
    }
    setReading(true);
    try {
      const content = await file.text();
      if (mounted.current) form.setValue("content", content, { shouldValidate: true });
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
    <form
      aria-label="混合运行输入"
      className="grid min-w-0 gap-4"
      onSubmit={form.handleSubmit((values) => {
        const input: WorkbenchRunInput = {
          scope: props.scope,
          recovery_enabled: values.recovery_enabled,
          content: values.content,
          template_id: local ? undefined : values.template_id || undefined,
          check_after_import: !local && !values.export_only && values.check_after_import,
          model: values.export_only ? "" : values.model,
          export_only: local || values.export_only,
          proxy_url: values.proxy_url || undefined,
          sms: oauthSMSInput(values),
        };
        form.reset({
          ...mixedRunDefaults,
          template_id: values.template_id,
          export_only: values.export_only,
        });
        props.onSubmit(input);
      })}
    >
      {!local && (
        <p className="text-sm text-muted-foreground">
          同一批可按顺序填写账号 JSON、rt_ 刷新令牌和邮箱登录行；JSON 数组会按项目展开。每批最多 500
          个账号。
        </p>
      )}
      <fieldset disabled={reading} className="grid min-w-0 gap-4">
        <div className="grid min-w-0 gap-3 sm:grid-cols-2">
          {!local && (
            <FormField label="处理方式">
              <Controller
                control={form.control}
                name="export_only"
                render={({ field }) => (
                  <Select
                    value={field.value ? "export" : "import"}
                    onValueChange={(value) => field.onChange(value === "export")}
                  >
                    <SelectTrigger aria-label="处理方式">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="import">导入线上账号</SelectItem>
                      <SelectItem value="export">生成私有 JSON</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            </FormField>
          )}
          <FormField label="从文件读取（最大 2 MB）" htmlFor="mixed-run-file">
            <Input
              id="mixed-run-file"
              type="file"
              accept=".txt,.json,.jsonl"
              disabled={reading}
              onChange={(event) => void loadFile(event)}
            />
          </FormField>
        </div>
        <FormField
          label="混合账号内容"
          htmlFor="mixed-run-content"
          error={form.formState.errors.content?.message}
        >
          <Textarea
            id="mixed-run-content"
            className="h-64 resize-y font-mono"
            autoComplete="off"
            spellCheck={false}
            aria-invalid={!!form.formState.errors.content}
            {...form.register("content")}
          />
        </FormField>
        <p className="text-xs text-muted-foreground">
          登录行可写为“邮箱----密码----2FA”，也可使用含 email、password、workspace_id、mailbox 的
          JSON 对象。提交预览后会清空输入，请先自行妥善保存原资料。
        </p>
        {!local && (
          <FormField label="配置模板">
            <Controller
              control={form.control}
              name="template_id"
              render={({ field }) => (
                <Select
                  value={field.value || "auto"}
                  onValueChange={(value) => field.onChange(value === "auto" ? "" : value)}
                >
                  <SelectTrigger aria-label="配置模板" className="min-w-0">
                    <SelectValue>
                      <span className="min-w-0 truncate">
                        {templates.data?.find((item) => item.id === field.value)?.name ??
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
        )}
        {!exportOnly && (
          <>
            <Controller
              control={form.control}
              name="check_after_import"
              render={({ field }) => (
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox checked={field.value} onCheckedChange={field.onChange} />
                  导入后执行模型检测
                </label>
              )}
            />
            <FormField
              label="检测模型"
              htmlFor="mixed-run-model"
              error={form.formState.errors.model?.message}
            >
              <Input
                id="mixed-run-model"
                disabled={!form.watch("check_after_import") || reading}
                aria-invalid={!!form.formState.errors.model}
                {...form.register("model")}
              />
            </FormField>
          </>
        )}
        <FormField
          label="本批登录代理"
          htmlFor="mixed-run-proxy"
          error={form.formState.errors.proxy_url?.message}
        >
          <Input
            id="mixed-run-proxy"
            type="password"
            autoComplete="off"
            aria-invalid={!!form.formState.errors.proxy_url}
            {...form.register("proxy_url")}
          />
        </FormField>
        <FormProvider {...form}>
          <WorkbenchOAuthSMSFields />
        </FormProvider>
        <Controller
          control={form.control}
          name="recovery_enabled"
          render={({ field }) => (
            <label className="flex items-start gap-2 text-sm">
              <Checkbox checked={field.value} onCheckedChange={field.onChange} />
              保存本批私有输入与结果以便中断恢复（最多 2 小时）
            </label>
          )}
        />
      </fieldset>
      {reading && <ContentLoading label="正在读取混合账号文件" compact />}
      <div>
        <Button type="submit" disabled={reading || (!local && templates.isError)}>
          解析混合运行范围
        </Button>
      </div>
    </form>
  );
}
