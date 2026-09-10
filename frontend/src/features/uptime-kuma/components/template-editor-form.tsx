import { requestEndpointURL, requestBodyFields } from "../lib/request-body";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ApiError, type KumaTemplateDetail } from "@/api";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import { notifyOperationError } from "@/lib/operation-feedback";
import { templateSchema, templateDefaults, type TemplateValues } from "../lib/template-schema";
import { TemplateMonitoringForm } from "./template-monitoring-form";
import { TemplateRequestForm } from "./template-request-form";

export function TemplateEditorForm(props: {
  item?: KumaTemplateDetail;
  pending: boolean;
  error: Error | null;
  onSubmit: (value: TemplateValues) => void;
}) {
  const form = useForm<TemplateValues>({
    resolver: zodResolver(templateSchema),
    defaultValues: templateDefaults(props.item),
  });
  const errors = form.formState.errors;
  useEffect(() => {
    if (!props.error) return;
    if (props.error instanceof ApiError && props.error.code === "kuma_invalid_template_name")
      form.setError("name", { message: props.error.message });
    else if (props.error instanceof ApiError && props.error.code === "kuma_invalid_template_auth")
      form.setError("auth_password", { message: props.error.message });
    else if (props.error instanceof ApiError && props.error.code === "kuma_invalid_template_model")
      form.setError("model", { message: props.error.message });
    else if (props.error instanceof ApiError && props.error.code === "kuma_invalid_template_body")
      form.setError("body", { message: props.error.message });
    else notifyOperationError(props.error, "模板保存失败");
  }, [form, props.error]);
  const submit = (value: TemplateValues): void => {
    const model = requestBodyFields(value.body).model;
    if (value.body_encoding === "json" && value.body && !value.clear_body) {
      try {
        JSON.parse(value.body);
      } catch {
        form.setError("body", { message: "请求体不是有效 JSON，请检查内容或切换编码" });
        return;
      }
    }
    if (value.request_profile && value.body_encoding === "json" && !model.trim()) {
      form.setError("model", { message: "请输入模型名称" });
      return;
    }
    props.onSubmit({
      ...value,
      model: model || value.model,
      monitoring: {
        ...value.monitoring,
        url: requestEndpointURL(value.monitoring.url, value.request_profile),
      },
      auth_method: "none",
      auth_username: "",
      auth_password: "",
      clear_auth: true,
      headers: value.clear_headers ? "" : value.headers,
      body: value.clear_body ? "" : value.body,
      clear_headers: value.clear_headers || (!!props.item?.headers && !value.headers),
      clear_body: value.clear_body || (!!props.item?.body && !value.body),
    });
  };
  return (
    <form id="kuma-template" onSubmit={form.handleSubmit(submit)}>
      <fieldset disabled={props.pending} className="grid min-w-0 gap-6">
        <FormField label="模板名称" htmlFor="template-name" error={errors.name?.message}>
          <Input id="template-name" {...form.register("name")} aria-invalid={!!errors.name} />
        </FormField>
        <TemplateMonitoringForm
          form={form}
          pending={props.pending}
          urlConfigured={!!props.item?.url_configured}
          editing={!!props.item}
        />
        <TemplateRequestForm form={form} item={props.item} pending={props.pending} />
      </fieldset>
    </form>
  );
}
