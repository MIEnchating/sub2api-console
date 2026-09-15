import type { ReactElement } from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { api, type WorkbenchScope } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { workbenchKeys } from "../constants";
import { oauthPreviewSchema, type OAuthPreviewValues } from "../lib/schemas";
import { usePreferredTemplate } from "../hooks/use-preferred-template";

export function WorkbenchOAuthOptions(props: {
  scope?: WorkbenchScope;
  disabled: boolean;
  onChange: () => void;
  onSubmit: (value: OAuthPreviewValues) => void;
}): ReactElement {
  const templates = useQuery({
    queryKey: workbenchKeys.templates,
    queryFn: api.workbenchTemplates,
    enabled: props.scope !== "local-export",
  });
  const form = useForm<OAuthPreviewValues>({
    resolver: zodResolver(oauthPreviewSchema),
    defaultValues: { template_id: "", check_after_import: false, model: "" },
  });
  usePreferredTemplate(props.scope === "local-export" ? undefined : templates.data, (id) =>
    form.setValue("template_id", id),
  );
  if (props.scope === "local-export")
    return (
      <form
        onSubmit={form.handleSubmit(() =>
          props.onSubmit({ template_id: "", check_after_import: false, model: "" }),
        )}
      >
        <Button type="submit" disabled={props.disabled}>
          预览授权账号
        </Button>
      </form>
    );
  if (templates.isPending) return <ContentLoading label="正在读取授权账号导入配置" />;
  if (!templates.data)
    return <ContentRetry onRetry={() => void templates.refetch()} pending={templates.isFetching} />;
  return (
    <form
      className="grid min-w-0 gap-4"
      onChange={props.onChange}
      onSubmit={form.handleSubmit(props.onSubmit)}
    >
      <div className="grid min-w-0 gap-4 sm:grid-cols-2">
        <FormField label="配置模板">
          <Controller
            control={form.control}
            name="template_id"
            render={({ field }) => (
              <Select
                value={field.value || "auto"}
                disabled={props.disabled}
                onValueChange={(value) => {
                  props.onChange();
                  field.onChange(value === "auto" ? "" : value);
                }}
              >
                <SelectTrigger aria-label="配置模板" className="min-w-0">
                  <SelectValue>
                    <span className="min-w-0 truncate">
                      {templates.data.find((item) => item.id === field.value)?.name ||
                        "自动匹配模板"}
                    </span>
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">自动匹配模板</SelectItem>
                  {templates.data.map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
        <FormField
          label="检测模型"
          htmlFor="workbench-oauth-model"
          error={form.formState.errors.model?.message}
        >
          <Input
            id="workbench-oauth-model"
            disabled={props.disabled || !form.watch("check_after_import")}
            aria-invalid={!!form.formState.errors.model}
            {...form.register("model")}
          />
        </FormField>
      </div>
      <Controller
        control={form.control}
        name="check_after_import"
        render={({ field }) => (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={field.value}
              disabled={props.disabled}
              onCheckedChange={(checked) => {
                props.onChange();
                field.onChange(checked);
              }}
            />
            导入后执行模型检测
          </label>
        )}
      />
      <div>
        <Button type="submit" disabled={props.disabled}>
          预览授权账号
        </Button>
      </div>
    </form>
  );
}
