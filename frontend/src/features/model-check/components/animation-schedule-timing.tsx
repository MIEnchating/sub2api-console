import type { ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { DailyDetectionTimes } from "./daily-detection-times";
import { FieldError } from "@/components/field-error";
import type { AnimationScheduleForm } from "../lib/animation-schema";

export function AnimationScheduleTiming(props: {
  form: UseFormReturn<AnimationScheduleForm>;
  pending: boolean;
}): ReactElement {
  const daily = props.form.watch("schedule_type") === "daily";
  const times = props.form.watch("daily_times") ?? [""];
  const timeError = props.form.formState.errors.daily_times?.message;
  return (
    <div className="space-y-3">
      <fieldset disabled={props.pending} className="space-y-2 text-sm">
        <legend>检测时间</legend>
        <div className="flex flex-wrap gap-4">
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name="schedule-time"
              checked={!daily}
              onChange={() => props.form.setValue("schedule_type", "interval")}
            />
            按间隔
          </label>
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name="schedule-time"
              checked={daily}
              onChange={() => {
                if (!Number.isInteger(props.form.getValues("interval_minutes"))) {
                  props.form.setValue("interval_minutes", 60);
                }
                props.form.setValue("schedule_type", "daily");
              }}
            />
            每天定时
          </label>
        </div>
      </fieldset>
      {daily ? (
        <DailyDetectionTimes
          times={times}
          pending={props.pending}
          error={timeError}
          onChange={(value) =>
            props.form.setValue("daily_times", value, { shouldDirty: true, shouldValidate: true })
          }
        />
      ) : (
        <>
          <label className="block space-y-1 text-sm">
            检测间隔（分钟）
            <Input
              type="number"
              min={1}
              max={1440}
              {...props.form.register("interval_minutes", { valueAsNumber: true })}
              disabled={props.pending}
              aria-invalid={!!props.form.formState.errors.interval_minutes}
            />
          </label>
          <FieldError message={props.form.formState.errors.interval_minutes?.message} />
          <p className="text-xs text-muted-foreground">
            本类检测结束后重新计时，服务器重启后重新等待设定间隔。
          </p>
        </>
      )}
    </div>
  );
}
