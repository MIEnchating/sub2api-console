import { TemplateAddressField } from "./template-address-field";
import { Controller, type UseFormReturn } from "react-hook-form";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { monitorTypeLabels } from "../constants";
import type { TemplateValues } from "../lib/template-schema";
import { MonitorFormSection } from "./monitor-form-section";

export function TemplateMonitoringForm(props: {
  form: UseFormReturn<TemplateValues>;
  pending: boolean;
  urlConfigured: boolean;
  editing: boolean;
}) {
  const form = props.form;
  const type = form.watch("monitoring.type");
  const http = ["http", "keyword"].includes(type);
  const errors = form.formState.errors.monitoring;
  return (
    <MonitorFormSection title="监控设置">
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField label="监控类型">
          <Controller
            control={form.control}
            name="monitoring.type"
            render={({ field }) => (
              <Select
                value={field.value}
                disabled={props.pending}
                onValueChange={(value) => {
                  if (!value) return;
                  field.onChange(value);
                  if (!["http", "keyword"].includes(value)) form.setValue("request_profile", "");
                }}
                itemToStringLabel={(value) => monitorTypeLabels[value] ?? value}
              >
                <SelectTrigger aria-label="监控类型">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(monitorTypeLabels).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
        {http && (
          <TemplateAddressField
            form={form}
            pending={props.pending}
            configured={props.urlConfigured}
          />
        )}
        {["port", "ping", "dns"].includes(type) && (
          <FormField
            label="主机名 / IP"
            htmlFor="template-hostname"
            error={errors?.hostname?.message}
          >
            <Input
              id="template-hostname"
              {...form.register("monitoring.hostname")}
              aria-invalid={!!errors?.hostname}
            />
          </FormField>
        )}
        {type === "port" && (
          <FormField label="TCP 端口" htmlFor="template-port" error={errors?.port?.message}>
            <Input
              id="template-port"
              type="number"
              {...form.register("monitoring.port", { valueAsNumber: true })}
            />
          </FormField>
        )}
        {type === "keyword" && (
          <FormField label="匹配关键字" htmlFor="template-keyword" error={errors?.keyword?.message}>
            <Input
              id="template-keyword"
              {...form.register("monitoring.keyword")}
              aria-invalid={!!errors?.keyword}
            />
          </FormField>
        )}
        {type === "dns" && (
          <>
            <FormField label="DNS 记录类型" htmlFor="template-dns-type">
              <Input id="template-dns-type" {...form.register("monitoring.dns_record_type")} />
            </FormField>
            <FormField label="DNS 解析服务器" htmlFor="template-dns-server">
              <Input id="template-dns-server" {...form.register("monitoring.dns_resolver")} />
            </FormField>
          </>
        )}
        {(
          [
            ["interval", "检测间隔（秒）", 20, 86400],
            ["timeout", "超时（秒）", 1, 3600],
            ["retry_interval", "重试间隔（秒）", 20, 86400],
            ["max_retries", "最大重试次数", 0, 100],
            ["max_redirects", "最大重定向次数", 0, 100],
          ] as const
        )
          .filter(([name]) => name !== "max_redirects" || http)
          .map(([name, label, min, max]) => (
            <FormField
              key={name}
              label={label}
              htmlFor={`template-${name}`}
              error={errors?.[name]?.message}
            >
              <Input
                id={`template-${name}`}
                type="number"
                min={min}
                max={max}
                {...form.register(`monitoring.${name}`, { valueAsNumber: true })}
                aria-invalid={!!errors?.[name]}
              />
            </FormField>
          ))}
        {http && (
          <FormField label="正常状态码" error={errors?.accepted_status_codes?.message}>
            <Controller
              control={form.control}
              name="monitoring.accepted_status_codes"
              render={({ field }) => (
                <Input
                  aria-label="正常状态码"
                  value={field.value.join(", ")}
                  onChange={(event) =>
                    field.onChange(event.target.value.split(",").map((value) => value.trim()))
                  }
                  aria-invalid={!!errors?.accepted_status_codes}
                />
              )}
            />
          </FormField>
        )}
        <div className="flex flex-wrap gap-4 sm:col-span-2">
          {(
            [
              ["ignore_tls", "忽略 TLS 证书错误"],
              ["upside_down", "反转正常 / 故障判断"],
            ] as const
          )
            .filter(([name]) => name !== "ignore_tls" || http)
            .map(([name, label]) => (
              <label key={name} className="flex items-center gap-2 text-sm">
                <Controller
                  control={form.control}
                  name={`monitoring.${name}`}
                  render={({ field }) => (
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={props.pending}
                    />
                  )}
                />
                {label}
              </label>
            ))}
          {http && props.editing && (
            <label className="flex items-center gap-2 text-sm">
              <Controller
                control={form.control}
                name="clear_url"
                render={({ field }) => (
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={props.pending}
                  />
                )}
              />
              清空模板监控地址
            </label>
          )}
        </div>
        {http && (
          <p className="text-xs text-muted-foreground sm:col-span-2">
            地址可留空，使用模板时填写。模板参数会带入监控表单，保存前仍可调整。
          </p>
        )}
      </div>
    </MonitorFormSection>
  );
}
