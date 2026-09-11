import { Controller, type UseFormReturn } from "react-hook-form";
import type { KumaMonitor, KumaTemplate } from "@/api";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import type { MonitorValues } from "../lib/schemas";

export function TemplateSelector(props: {
  form: UseFormReturn<MonitorValues>;
  templates: KumaTemplate[];
  disabled: boolean;
  editing?: boolean;
  monitor?: KumaMonitor | null;
}) {
  const selectedID = props.form.watch("template_id");
  const retained = !!props.form.watch("template_retain");
  const selected = props.templates.find((item) => item.id === selectedID);
  const selectedName = retained ? props.monitor?.template_name : undefined;
  const selectTemplate = (id: string | null): void => {
    const item = props.templates.find((value) => value.id === id);
    props.form.setValue("template_id", item?.id ?? "");
    props.form.setValue("template_model", "");
    props.form.setValue("template_retain", false);
    props.form.setValue("template_clear", !item);
    props.form.clearErrors("template_model");
    props.form.setValue("template_revision", item?.revision ?? 0);
    props.form.setValue("template_settings_override", !!item?.monitoring);
    if (item && !item.monitoring && !props.editing) props.form.setValue("type", "http");
    if (item?.monitoring) {
      const m = item.monitoring;
      if (!props.editing) props.form.setValue("type", m.type);
      props.form.setValue("interval", m.interval);
      if (m.url) props.form.setValue("url", m.url);
      else if (item.url_redacted) props.form.setValue("url", "");
      for (const name of [
        "timeout",
        "retry_interval",
        "max_retries",
        "max_redirects",
        "accepted_status_codes",
        "ignore_tls",
        "upside_down",
        "hostname",
        "port",
        "keyword",
        "dns_record_type",
        "dns_resolver",
      ] as const) {
        props.form.setValue(`options.${name}`, m[name]);
      }
    }
    const authMethod = props.form.getValues("options.auth_method") ?? "none";
    props.form.setValue(
      "template_auth_override",
      !!item && (authMethod !== "none" || !!props.form.getValues("template_auth_override")),
    );
    if (item) {
      for (const field of ["headers", "body"] as const) props.form.setValue(`options.${field}`, "");
      props.form.clearErrors("options.headers");
    }
  };
  return (
    <div className="grid gap-2">
      <FormField label="功能模板">
        <Controller
          control={props.form.control}
          name="template_id"
          render={({ field }) => (
            <Select
              value={field.value || "manual"}
              disabled={props.disabled}
              itemToStringLabel={(id) =>
                (id === selectedID && selectedName) ||
                props.templates.find((item) => item.id === id)?.name ||
                "手动设置"
              }
              onValueChange={selectTemplate}
            >
              <SelectTrigger aria-label="功能模板">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="manual">手动设置</SelectItem>
                {props.templates
                  .filter(
                    (item) =>
                      !props.editing ||
                      (item.monitoring
                        ? item.monitoring.type === props.form.getValues("type")
                        : ["http", "keyword"].includes(props.form.getValues("type"))),
                  )
                  .map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.name}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          )}
        />
      </FormField>
      {selected?.body_configured &&
        ["http", "keyword"].includes(props.form.watch("type")) &&
        (!selected.body_encoding || selected.body_encoding === "json") && (
          <FormField
            label="请求模型"
            htmlFor="kuma-template-model"
            error={props.form.formState.errors.template_model?.message}
          >
            <Input
              id="kuma-template-model"
              {...props.form.register("template_model")}
              placeholder={selected.model || "使用模板模型"}
              disabled={props.disabled}
              aria-invalid={!!props.form.formState.errors.template_model}
            />
          </FormField>
        )}
      {selected && (
        <p role="status" className="text-xs leading-relaxed text-muted-foreground">
          {selected.method} · 请求头{selected.headers_configured ? "已配置" : "为空"} · 请求体
          {selected.body_configured ? "已配置" : "为空"}
        </p>
      )}
    </div>
  );
}
