import type { ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldError } from "@/components/field-error";
import { MultiSelect } from "@/components/multi-select";
import type { GroupStatus } from "@/api";
import type { DetectionTaskForm } from "../lib/detection-task-schema";
import { DailyDetectionTimes } from "./daily-detection-times";
import { DetectionTaskStages } from "./detection-task-stages";

export function DetectionTaskFields(props: {
  form: UseFormReturn<DetectionTaskForm>;
  groups: GroupStatus[];
  pending: boolean;
}): ReactElement {
  const form = props.form;
  const errors = form.formState.errors;
  const groupOptions = props.groups.flatMap((group) =>
    group.id ? [{ value: group.id, label: `${group.name}（ID ${group.id}）` }] : [],
  );
  return (
    <div className="min-w-0 space-y-5">
      <section aria-label="任务范围" className="min-w-0 space-y-3">
        <label className="block space-y-1 text-sm">
          任务名称
          <Input {...form.register("name")} disabled={props.pending} aria-invalid={!!errors.name} />
        </label>
        <FieldError message={errors.name?.message} />
        <div className="space-y-1 text-sm">
          <p>检测分组</p>
          <MultiSelect
            ariaLabel="检测分组"
            options={groupOptions}
            selected={form.watch("group_ids")}
            onChange={(value) => form.setValue("group_ids", value, { shouldValidate: true })}
            disabled={props.pending}
            ariaInvalid={!!errors.group_ids}
            unknownValueLabel="分组已删除"
          />
          <FieldError message={errors.group_ids?.message} />
        </div>
        <p className="text-xs text-muted-foreground">
          执行时读取所选分组的最新账号，多分组中的同一账号只检测一次。
        </p>
      </section>
      <div className="grid min-w-0 gap-4 sm:grid-cols-[minmax(0,1fr)_12rem]">
        <div className="min-w-0 space-y-1">
          <label className="block space-y-1 text-sm">
            检测模型
            <Input
              {...form.register("model")}
              disabled={props.pending}
              aria-invalid={!!errors.model}
            />
          </label>
          <FieldError message={errors.model?.message} />
          <p className="text-xs text-muted-foreground">前置检测：medium · 鹈鹕动画：low</p>
        </div>
        <div className="min-w-0 space-y-1">
          <label className="block space-y-1 text-sm">
            请求超时（秒）
            <Input
              type="number"
              min={5}
              max={120}
              {...form.register("timeout_seconds", { valueAsNumber: true })}
              disabled={props.pending}
              aria-invalid={!!errors.timeout_seconds}
            />
          </label>
          <FieldError message={errors.timeout_seconds?.message} />
        </div>
      </div>
      <DetectionTaskStages form={form} pending={props.pending} />
      <section aria-label="自动检测计划" className="space-y-3 border-t pt-4">
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={form.watch("automatic")}
            onCheckedChange={(value) => form.setValue("automatic", value)}
            disabled={props.pending}
          />
          自动检测
        </label>
        <fieldset disabled={props.pending} className="flex gap-4 text-sm">
          <legend className="mb-2">自动检测时间</legend>
          {(["interval", "daily"] as const).map((type) => (
            <label key={type} className="flex items-center gap-2">
              <input
                type="radio"
                name="task-schedule-type"
                checked={form.watch("schedule_type") === type}
                onChange={() => form.setValue("schedule_type", type)}
              />
              {type === "daily" ? "每天定时" : "按间隔"}
            </label>
          ))}
        </fieldset>
        {form.watch("schedule_type") === "daily" ? (
          <DailyDetectionTimes
            times={form.watch("daily_times")}
            onChange={(value) => form.setValue("daily_times", value, { shouldValidate: true })}
            pending={props.pending}
            error={errors.daily_times?.message}
          />
        ) : (
          <>
            <label className="block space-y-1 text-sm">
              检测间隔（分钟）
              <Input
                type="number"
                min={1}
                max={1440}
                {...form.register("interval_minutes", { valueAsNumber: true })}
                disabled={props.pending}
                aria-invalid={!!errors.interval_minutes}
              />
            </label>
            <FieldError message={errors.interval_minutes?.message} />
          </>
        )}
      </section>
    </div>
  );
}
