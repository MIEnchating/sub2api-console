import { z } from "zod";
import type { AccountModelAutomationSchedule } from "@/api";

export const automationScheduleNameSchema = z
  .string()
  .trim()
  .min(1, "请输入任务名称")
  .refine((name) => [...name].length <= 128, "任务名称不能超过 128 个字符");

export const automationTimingSchema = z
  .object({
    schedule_type: z.enum(["interval", "daily"]),
    interval_minutes: z.string(),
    daily_times: z.array(z.string()),
  })
  .superRefine((value, context) => {
    if (value.schedule_type === "interval") {
      const interval = Number(value.interval_minutes);
      if (!Number.isInteger(interval) || interval < 1 || interval > 10080) {
        context.addIssue({
          code: "custom",
          path: ["interval_minutes"],
          message: "运行间隔需为 1 到 10080 分钟之间的整数。",
        });
      }
      return;
    }
    if (
      value.daily_times.length < 1 ||
      value.daily_times.length > 24 ||
      value.daily_times.some((time) => !/^([01]\d|2[0-3]):[0-5]\d$/.test(time))
    ) {
      context.addIssue({
        code: "custom",
        path: ["daily_times"],
        message: "每天请选择 1～24 个有效运行时间。",
      });
    } else if (new Set(value.daily_times).size !== value.daily_times.length) {
      context.addIssue({ code: "custom", path: ["daily_times"], message: "运行时间不能重复。" });
    }
  });

export type AutomationTimingForm = z.infer<typeof automationTimingSchema>;

export function automationScheduleLabel(schedule: AccountModelAutomationSchedule): string {
  if (schedule.schedule_type === "daily") {
    return `每天 ${(schedule.daily_times ?? []).join("、")}（北京时间）`;
  }
  return `每 ${schedule.interval_minutes} 分钟`;
}
