import type { ReactElement } from "react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { fingerprintLabels, subscriptionLabels } from "../constants";
import type { ManualTemplateValues } from "../lib/manual-template-schema";

type SettingsProps = { form: UseFormReturn<ManualTemplateValues>; pending: boolean };
const schedulingFields = [
  { name: "concurrency", label: "并发数", type: "number", hint: "0 表示不限流" },
  { name: "priority", label: "优先级", type: "number", hint: "请输入非负整数" },
  { name: "rate_multiplier", label: "计费倍率", type: "text", hint: "例如：1 或 0.5" },
  { name: "load_factor", label: "负载因子", type: "text", hint: "留空使用默认值" },
] as const;
const bindingFields = [
  { name: "proxy_id", label: "代理 ID", type: "text", hint: "留空直连" },
  { name: "group_ids", label: "分组 ID", type: "text", hint: "多个 ID 用逗号分隔" },
] as const;
type InputField = (typeof schedulingFields)[number] | (typeof bindingFields)[number];

function ConfigInput(props: SettingsProps & { field: InputField }): ReactElement {
  const field = props.field;
  const error = props.form.formState.errors[field.name];
  return (
    <div className="grid min-w-0 content-start gap-1.5">
      <label htmlFor={`manual-${field.name}`} className="text-sm">
        {field.label}
      </label>
      <Input
        id={`manual-${field.name}`}
        type={field.type}
        min={field.type === "number" ? 0 : undefined}
        placeholder={field.hint}
        aria-invalid={!!error}
        aria-describedby={`manual-${field.name}-hint`}
        {...props.form.register(field.name, { valueAsNumber: field.type === "number" })}
      />
      <p id={`manual-${field.name}-hint`} className="text-xs text-muted-foreground">
        {field.hint}
      </p>
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error.message}
        </p>
      )}
    </div>
  );
}

export function TemplateSettingsFields(props: SettingsProps): ReactElement {
  return (
    <>
      <fieldset
        disabled={props.pending}
        className="grid min-w-0 gap-4 rounded-lg border p-4 sm:grid-cols-2"
      >
        <legend className="px-1 text-sm font-medium">调度与计费</legend>
        {schedulingFields.map((field) => (
          <ConfigInput key={field.name} {...props} field={field} />
        ))}
      </fieldset>
      <fieldset
        disabled={props.pending}
        className="grid min-w-0 gap-4 rounded-lg border p-4 sm:grid-cols-2"
      >
        <legend className="px-1 text-sm font-medium">账号配置</legend>
        {bindingFields.map((field) => (
          <ConfigInput key={field.name} {...props} field={field} />
        ))}
        <div className="grid min-w-0 content-start gap-1.5">
          <label htmlFor="manual-plan" className="text-sm">
            订阅档位
          </label>
          <Controller
            name="plan"
            control={props.form.control}
            render={({ field }) => (
              <Select
                disabled={props.pending}
                value={field.value || "auto"}
                onValueChange={(value) => field.onChange(value === "auto" ? "" : value)}
              >
                <SelectTrigger id="manual-plan">
                  <SelectValue>{subscriptionLabels[field.value] || "自动识别"}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">自动识别</SelectItem>
                  {Object.entries(subscriptionLabels).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </div>
        <div className="grid min-w-0 content-start gap-1.5">
          <label htmlFor="manual-fingerprint" className="text-sm">
            Codex 指纹
          </label>
          <Controller
            name="fingerprint"
            control={props.form.control}
            render={({ field }) => (
              <Select disabled={props.pending} value={field.value} onValueChange={field.onChange}>
                <SelectTrigger id="manual-fingerprint">
                  <SelectValue>{fingerprintLabels[field.value]}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(fingerprintLabels).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </div>
        <label className="flex items-center gap-2 border-t pt-3 text-sm sm:col-span-2">
          <Controller
            name="auto_pause_on_expired"
            control={props.form.control}
            render={({ field }) => (
              <Checkbox
                disabled={props.pending}
                checked={field.value}
                onCheckedChange={field.onChange}
              />
            )}
          />
          到期自动暂停
        </label>
      </fieldset>
    </>
  );
}
