import { Controller, type UseFormReturn } from "react-hook-form";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import { MultiSelect } from "@/components/multi-select";
import type { KumaResourceList } from "@/api";
import type { ResourceValues } from "../lib/resource-schemas";
import { maintenanceStrategies } from "../constants";
import { ResourceTextField, ResourceSelectField, ResourceCheckboxField } from "./resource-fields";
export function MaintenanceForm(props: {
  form: UseFormReturn<ResourceValues>;
  options: KumaResourceList;
  pending: boolean;
}) {
  const strategy = props.form.watch("maintenance.strategy");
  const common = { form: props.form };
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <ResourceTextField {...common} name="maintenance.title" label="维护标题" />
      <ResourceSelectField
        {...common}
        name="maintenance.strategy"
        label="维护策略"
        options={maintenanceStrategies}
        disabled={props.pending}
      />
      <ResourceTextField {...common} name="maintenance.description" label="维护说明" multiline />
      <ResourceTextField
        {...common}
        name="maintenance.timezone"
        label="时区"
        placeholder="Asia/Shanghai 或 SAME_AS_SERVER"
      />
      {strategy !== "manual" && (
        <>
          <ResourceTextField
            {...common}
            name="maintenance.start"
            label="开始日期时间"
            type="datetime-local"
          />
          <ResourceTextField
            {...common}
            name="maintenance.end"
            label="结束日期时间"
            type="datetime-local"
          />
        </>
      )}
      {strategy === "cron" && (
        <>
          <ResourceTextField {...common} name="maintenance.cron" label="Cron 表达式" />
          <ResourceTextField
            {...common}
            name="maintenance.duration_minutes"
            label="持续时间（分钟）"
            type="number"
          />
        </>
      )}
      {strategy.startsWith("recurring-") && (
        <>
          <ResourceTextField
            {...common}
            name="maintenance.start_time"
            label="每日开始时间"
            type="time"
          />
          <ResourceTextField
            {...common}
            name="maintenance.end_time"
            label="每日结束时间"
            type="time"
          />
        </>
      )}
      {strategy === "recurring-interval" && (
        <ResourceTextField
          {...common}
          name="maintenance.interval_days"
          label="重复间隔（天）"
          type="number"
        />
      )}
      {strategy === "recurring-weekday" && (
        <FormField label="星期" error={props.form.formState.errors.maintenance?.weekdays?.message}>
          <Controller
            control={props.form.control}
            name="maintenance.weekdays"
            render={({ field }) => (
              <MultiSelect
                ariaLabel="星期"
                title="选择星期"
                options={["周日", "周一", "周二", "周三", "周四", "周五", "周六"].map(
                  (label, i) => ({ label, value: String(i) }),
                )}
                selected={field.value.map(String)}
                onChange={(v) => field.onChange(v.map(Number))}
                disabled={props.pending}
              />
            )}
          />
        </FormField>
      )}
      {strategy === "recurring-day-of-month" && (
        <FormField
          label="每月日期（逗号分隔）"
          error={props.form.formState.errors.maintenance?.days_of_month?.message}
        >
          <Controller
            control={props.form.control}
            name="maintenance.days_of_month"
            render={({ field }) => (
              <Input
                aria-label="每月日期（逗号分隔）"
                value={field.value.join(",")}
                onChange={(e) =>
                  field.onChange(e.target.value ? e.target.value.split(",").map(Number) : [])
                }
              />
            )}
          />
        </FormField>
      )}
      {strategy === "recurring-day-of-month" && (
        <ResourceCheckboxField
          {...common}
          name="maintenance.last_day"
          label="包含每月最后一天"
          disabled={props.pending}
        />
      )}
      <FormField label="维护监控项">
        <Controller
          control={props.form.control}
          name="maintenance.monitor_ids"
          render={({ field }) => (
            <MultiSelect
              ariaLabel="维护监控项"
              title="选择监控项"
              options={props.options.monitors.map((m) => ({ label: m.name, value: String(m.id) }))}
              selected={field.value.map(String)}
              onChange={(v) => field.onChange(v.map(Number))}
              disabled={props.pending}
            />
          )}
        />
      </FormField>
      <FormField label="展示维护的状态页">
        <Controller
          control={props.form.control}
          name="maintenance.status_page_ids"
          render={({ field }) => (
            <MultiSelect
              ariaLabel="展示维护的状态页"
              title="选择状态页"
              options={props.options.status_pages.map((p) => ({
                label: p.title,
                value: String(p.id),
              }))}
              selected={field.value.map(String)}
              onChange={(v) => field.onChange(v.map(Number))}
              disabled={props.pending}
            />
          )}
        />
      </FormField>
      <ResourceCheckboxField
        {...common}
        name="maintenance.active"
        label="启用维护计划"
        disabled={props.pending}
      />
    </div>
  );
}
