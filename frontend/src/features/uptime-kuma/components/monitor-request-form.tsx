import { Controller, type UseFormReturn } from "react-hook-form";
import type { KumaMonitor } from "@/api";
import { FormField } from "@/App";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { MonitorValues } from "../lib/schemas";
import { httpMethods } from "../constants";
import { MonitorFormSection } from "./monitor-form-section";

export function MonitorRequestForm(props: {
  form: UseFormReturn<MonitorValues>;
  monitor: KumaMonitor | null;
  pending: boolean;
}) {
  const form = props.form;
  const usingTemplate = !!form.watch("template_id");
  const errors = form.formState.errors.options;
  if (usingTemplate) return null;
  return (
    <MonitorFormSection title="请求设置">
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField label="请求方法">
          <Controller
            control={form.control}
            name="options.method"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange} disabled={props.pending}>
                <SelectTrigger aria-label="请求方法">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {httpMethods.map((v) => (
                    <SelectItem key={v} value={v}>
                      {v}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField
          label={props.monitor ? "请求头（JSON，留空保留）" : "请求头（JSON）"}
          htmlFor="kuma-headers"
          error={errors?.headers?.message}
        >
          <JsonEditorField
            id="kuma-headers"
            aria-label={props.monitor ? "请求头（JSON，留空保留）" : "请求头（JSON）"}
            control={form.control}
            name="options.headers"
            disabled={props.pending}
            placeholder={
              props.monitor?.options?.headers_configured
                ? "已配置，留空保留"
                : '{"Content-Type":"application/json"}'
            }
            aria-invalid={!!errors?.headers}
          />
        </FormField>
        <FormField
          label={props.monitor ? "请求体（留空保留）" : "请求体"}
          htmlFor="kuma-body"
          error={errors?.body?.message}
        >
          <JsonEditorField
            id="kuma-body"
            aria-label={props.monitor ? "请求体（留空保留）" : "请求体"}
            control={form.control}
            name="options.body"
            disabled={props.pending}
            language="auto"
            placeholder={props.monitor?.options?.body_configured ? "已配置，留空保留" : "请求体"}
          />
        </FormField>
      </div>
      {props.monitor && (
        <div className="flex flex-wrap gap-4">
          {(
            [
              ["clear_headers", "清空已有请求头"],
              ["clear_body", "清空已有请求体"],
            ] as const
          ).map(([name, label]) => (
            <label key={name} className="flex items-center gap-2 text-sm">
              <Controller
                control={form.control}
                name={`options.${name}`}
                render={({ field }) => (
                  <Checkbox
                    checked={!!field.value}
                    onCheckedChange={field.onChange}
                    disabled={props.pending}
                  />
                )}
              />
              {label}
            </label>
          ))}
        </div>
      )}
    </MonitorFormSection>
  );
}
