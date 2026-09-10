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
import type { KumaMonitor } from "@/api";
import type { MonitorValues } from "../lib/schemas";

import { MonitorFormSection } from "./monitor-form-section";

type Props = {
  form: UseFormReturn<MonitorValues>;
  monitor: KumaMonitor | null;
  pending: boolean;
};
export function MonitorOptionsForm(props: Props) {
  const form = props.form;
  const type = form.watch("type");
  const http = ["http", "keyword"].includes(type);
  const errors = form.formState.errors.options;
  return (
    <MonitorFormSection title="检测设置">
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField
          htmlFor="kuma-monitor-interval"
          label="检测间隔（秒）"
          error={form.formState.errors.interval?.message}
        >
          <Input
            type="number"
            min={20}
            max={86400}
            id="kuma-monitor-interval"
            {...form.register("interval", { valueAsNumber: true })}
            aria-invalid={!!form.formState.errors.interval}
          />
        </FormField>

        {["port", "ping", "dns"].includes(type) && (
          <FormField label="主机名 / IP" htmlFor="kuma-hostname" error={errors?.hostname?.message}>
            <Input
              id="kuma-hostname"
              {...form.register("options.hostname")}
              aria-invalid={!!errors?.hostname}
            />
          </FormField>
        )}
        {type === "port" && (
          <FormField label="TCP 端口" htmlFor="kuma-port" error={errors?.port?.message}>
            <Input
              id="kuma-port"
              type="number"
              min={1}
              max={65535}
              {...form.register("options.port", { valueAsNumber: true })}
              aria-invalid={!!errors?.port}
            />
          </FormField>
        )}
        {type === "keyword" && (
          <FormField label="匹配关键字" htmlFor="kuma-keyword" error={errors?.keyword?.message}>
            <Input
              id="kuma-keyword"
              {...form.register("options.keyword")}
              aria-invalid={!!errors?.keyword}
            />
          </FormField>
        )}
        {type === "dns" && (
          <>
            <FormField label="DNS 记录类型">
              <Controller
                control={form.control}
                name="options.dns_record_type"
                render={({ field }) => (
                  <Select
                    value={field.value}
                    onValueChange={field.onChange}
                    disabled={props.pending}
                  >
                    <SelectTrigger aria-label="DNS 记录类型">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {["A", "AAAA", "CNAME", "MX", "NS", "TXT", "SRV", "PTR", "SOA", "CAA"].map(
                        (v) => (
                          <SelectItem key={v} value={v}>
                            {v}
                          </SelectItem>
                        ),
                      )}
                    </SelectContent>
                  </Select>
                )}
              />
            </FormField>
            <FormField label="DNS 解析服务器" htmlFor="kuma-dns">
              <Input id="kuma-dns" {...form.register("options.dns_resolver")} />
            </FormField>
          </>
        )}
        {http && (
          <>
            <FormField label="正常状态码" error={errors?.accepted_status_codes?.message}>
              <Controller
                control={form.control}
                name="options.accepted_status_codes"
                render={({ field }) => (
                  <Input
                    aria-label="正常状态码"
                    value={field.value?.join(", ") ?? ""}
                    onChange={(e) => field.onChange(e.target.value.split(",").map((v) => v.trim()))}
                    placeholder="200-299, 301"
                    aria-invalid={!!errors?.accepted_status_codes}
                  />
                )}
              />
            </FormField>
          </>
        )}
        {type !== "group" &&
          (
            [
              ["timeout", "超时（秒）", 1, 3600],
              ["retry_interval", "重试间隔（秒）", 20, 86400],
              ["max_retries", "最大重试次数", 0, 100],
            ] as const
          ).map(([name, label, min, max]) => (
            <FormField
              key={name}
              label={label}
              htmlFor={`kuma-${name}`}
              error={errors?.[name]?.message}
            >
              <Input
                id={`kuma-${name}`}
                type="number"
                min={min}
                max={max}
                {...form.register(`options.${name}`, { valueAsNumber: true })}
                aria-invalid={!!errors?.[name]}
              />
            </FormField>
          ))}
        {http && (
          <FormField
            label="最大重定向次数"
            htmlFor="kuma-redirects"
            error={errors?.max_redirects?.message}
          >
            <Input
              id="kuma-redirects"
              type="number"
              min={0}
              max={100}
              {...form.register("options.max_redirects", { valueAsNumber: true })}
            />
          </FormField>
        )}
        <div className="flex flex-wrap gap-4 sm:col-span-2">
          {(
            [
              ...(http ? ([["ignore_tls", "忽略 TLS 证书错误"]] as const) : []),
              ["upside_down", "反转正常 / 故障判断"],
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
      </div>
    </MonitorFormSection>
  );
}
