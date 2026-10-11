import type { ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Plus, Trash2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { automationTimingSchema, type AutomationTimingForm } from "../lib/automation-schedule";

export function AutomationScheduleTiming(props: {
  form: UseFormReturn<AutomationTimingForm>;
  pending: boolean;
}): ReactElement {
  const value = props.form.watch();
  const validation = automationTimingSchema.safeParse(value);
  const error = validation.success ? undefined : validation.error.issues[0]?.message;
  const daily = value.schedule_type === "daily";
  const setTimes = (times: string[]): void => {
    props.form.setValue("daily_times", times, { shouldDirty: true, shouldValidate: true });
  };
  return (
    <fieldset
      disabled={props.pending}
      className="grid min-w-0 content-start items-start gap-3 text-sm"
    >
      <legend className="sr-only">运行方式</legend>
      <div className="flex min-h-8 flex-wrap items-center gap-x-3 gap-y-2">
        <label className="flex items-center gap-2">
          <input type="radio" value="interval" {...props.form.register("schedule_type")} />
          按间隔
        </label>
        <label className="flex items-center gap-2">
          <input type="radio" value="daily" {...props.form.register("schedule_type")} />
          指定时段
        </label>
      </div>
      {daily ? (
        <div className="grid min-w-0 gap-2">
          <p>每天运行时间（北京时间）</p>
          <div className="flex max-h-32 flex-wrap items-start gap-2 overflow-y-auto">
            {value.daily_times.map((time, index) => (
              <div key={index} className="flex w-44 min-w-0 items-center gap-1">
                <Input
                  type="time"
                  step={60}
                  value={time}
                  aria-label={index === 0 ? "每天运行时间" : `每天运行时间 ${index + 1}`}
                  aria-invalid={!!error}
                  onChange={(event) =>
                    setTimes(
                      value.daily_times.map((item, position) =>
                        position === index ? event.target.value : item,
                      ),
                    )
                  }
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={`删除每天运行时间 ${index + 1}`}
                  disabled={value.daily_times.length === 1 || props.pending}
                  onClick={() =>
                    setTimes(value.daily_times.filter((_, position) => position !== index))
                  }
                >
                  <Trash2 aria-hidden="true" />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              className="w-fit"
              disabled={value.daily_times.length >= 24 || props.pending}
              onClick={() => setTimes([...value.daily_times, ""])}
            >
              <Plus aria-hidden="true" />
              添加时间
            </Button>
          </div>
        </div>
      ) : (
        <label className="flex flex-wrap items-center gap-2">
          运行间隔（分钟）
          <Input
            type="number"
            className="w-24"
            min={1}
            max={10080}
            aria-label="运行间隔分钟"
            aria-invalid={!!error}
            {...props.form.register("interval_minutes")}
          />
        </label>
      )}
      {error && <p className="text-xs text-destructive">{error}</p>}
    </fieldset>
  );
}
