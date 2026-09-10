import { Controller, type UseFormReturn } from "react-hook-form";
import type { KumaMonitor } from "@/api";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { monitorTypeLabels } from "../constants";
import type { MonitorValues } from "../lib/schemas";
import { MonitorFormSection } from "./monitor-form-section";

export function MonitorBasicsForm(props: {
  form: UseFormReturn<MonitorValues>;
  monitor: KumaMonitor | null;
  monitors: KumaMonitor[];
  pending: boolean;
}) {
  const form = props.form;
  const type = form.watch("type");
  return (
    <MonitorFormSection title="基本信息">
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField
          htmlFor="kuma-monitor-name"
          label="监控项名称"
          error={form.formState.errors.name?.message}
        >
          <Input
            id="kuma-monitor-name"
            {...form.register("name")}
            aria-invalid={!!form.formState.errors.name}
          />
        </FormField>
        <FormField htmlFor="kuma-monitor-type" label="监控类型">
          <Controller
            control={form.control}
            name="type"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value);
                  form.setValue("template_id", "");
                  form.setValue("template_revision", 0);
                  form.setValue("template_settings_override", false);
                  form.setValue("template_auth_override", false);
                }}
                disabled={props.pending || !!props.monitor}
                itemToStringLabel={(value) => monitorTypeLabels[value] ?? value}
              >
                <SelectTrigger id="kuma-monitor-type" onBlur={field.onBlur} ref={field.ref}>
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
        {["http", "keyword"].includes(type) && (
          <FormField
            htmlFor="kuma-monitor-url"
            label={props.monitor ? "新监控地址（留空保留原地址）" : "监控地址"}
            error={form.formState.errors.url?.message}
          >
            <Input
              id="kuma-monitor-url"
              {...form.register("url")}
              placeholder="https://api.example.com/health"
              aria-invalid={!!form.formState.errors.url}
            />
          </FormField>
        )}
        <FormField
          htmlFor="kuma-monitor-parent"
          label="所属分组"
          error={form.formState.errors.parent?.message}
        >
          <Controller
            control={form.control}
            name="parent"
            render={({ field }) => (
              <Select
                value={field.value ?? 0}
                onValueChange={(value) => field.onChange(value || null)}
                disabled={props.pending}
                itemToStringLabel={(value) =>
                  props.monitors.find((monitor) => monitor.id === value)?.name ?? "无分组"
                }
              >
                <SelectTrigger
                  id="kuma-monitor-parent"
                  onBlur={field.onBlur}
                  ref={field.ref}
                  aria-invalid={!!form.formState.errors.parent}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={0}>无分组</SelectItem>
                  {props.monitors
                    .filter(
                      (monitor) => monitor.type === "group" && monitor.id !== props.monitor?.id,
                    )
                    .map((monitor) => (
                      <SelectItem key={monitor.id} value={monitor.id}>
                        {monitor.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
      </div>
    </MonitorFormSection>
  );
}
