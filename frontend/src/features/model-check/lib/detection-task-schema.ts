import { z } from "zod";
import { animationScheduleSchema } from "./animation-schema";
import { detectionTaskStages } from "./detection-task-stages";

export const terminalRoundsSchema = z.object({
  rounds: z.number().int().min(1, "至少检测 1 轮").max(20, "最多检测 20 轮"),
});
export const detectionTaskSchema = z
  .object({
    id: z.string(),
    version: z.number().int().min(0),
    name: z
      .string()
      .trim()
      .min(1, "请输入任务名称")
      .max(80, "任务名称不能超过 80 个字符")
      .refine((value) => !/\p{Cc}/u.test(value), "任务名称不能包含控制字符"),
    group_ids: z
      .array(z.string().regex(/^[1-9]\d*$/))
      .min(1, "请选择至少一个分组")
      .max(100, "最多选择 100 个分组"),
    model: z.string(),
    animation: z.boolean(),
    precheck: z.boolean(),
    precheck_questions: z.array(z.enum(["candy"])),
    terminal: z.boolean(),
    terminal_rounds: z.union([z.number(), z.nan()]),
    automatic: z.boolean(),
    schedule_type: z.enum(["interval", "daily"]),
    interval_minutes: z.number(),
    daily_times: z.array(z.string()),
    timeout_seconds: z.number(),
  })
  .superRefine((value, ctx) => {
    if (!value.animation && !value.precheck && !value.terminal)
      ctx.addIssue({ code: "custom", path: ["animation"], message: "请至少选择一项检测内容" });
    if (value.terminal) {
      const rounds = terminalRoundsSchema.safeParse({ rounds: value.terminal_rounds });
      if (!rounds.success)
        ctx.addIssue({
          code: "custom",
          path: ["terminal_rounds"],
          message: "终端检测轮数必须在 1 到 20 之间",
        });
    }
    const check = animationScheduleSchema.safeParse({
      ...value,
      account_id: "1",
      enabled: true,
      mode: value.precheck ? "precheck" : "animation",
      timezone: value.schedule_type === "daily" ? "Asia/Shanghai" : undefined,
    });
    if (!check.success)
      for (const issue of check.error.issues)
        ctx.addIssue({ code: "custom", path: issue.path, message: issue.message });
  })
  .transform((value) => ({
    ...value,
    terminal_rounds: terminalRoundsSchema.shape.rounds.safeParse(value.terminal_rounds).success
      ? value.terminal_rounds
      : 3,
  }));
export type DetectionTaskForm = z.infer<typeof detectionTaskSchema>;

export function detectionTaskDescription(value: DetectionTaskForm): string {
  const timing =
    value.schedule_type === "daily"
      ? "每天 " + value.daily_times.join("、") + "（北京时间）"
      : "每 " + value.interval_minutes + " 分钟";
  return timing + "依次执行" + detectionTaskStages(value).join(" → ");
}
