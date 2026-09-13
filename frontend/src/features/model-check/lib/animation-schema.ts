import { z } from "zod";

const modelID = z
  .string()
  .trim()
  .min(1, "请输入模型 ID")
  .max(256, "模型 ID 不能超过 256 个字符")
  .refine((value) => !/\p{Cc}/u.test(value), "模型 ID 不能包含控制字符");
export const animationSchema = z.object({
  account_ids: z
    .array(z.string().regex(/^[1-9]\d*$/))
    .min(1, "请选择至少一个账号")
    .max(20, "单次最多检测 20 个账号"),
  unified_model: modelID,
  timeout_seconds: z.number().int().min(5, "超时不能小于 5 秒").max(120, "超时不能超过 120 秒"),
});
export type AnimationForm = z.infer<typeof animationSchema>;

export const animationScheduleSchema = z
  .object({
    account_id: z.string().regex(/^[1-9]\d*$/),
    enabled: z.boolean(),
    model: z.string(),
    interval_minutes: z
      .number()
      .int()
      .min(1, "间隔不能小于 1 分钟")
      .max(1440, "间隔不能超过 1440 分钟"),
    timeout_seconds: z.number().int().min(5, "超时不能小于 5 秒").max(120, "超时不能超过 120 秒"),
    version: z.number().int().min(0),
  })
  .superRefine((value, context) => {
    if (value.enabled) {
      const parsed = modelID.safeParse(value.model);
      if (!parsed.success)
        context.addIssue({
          code: "custom",
          path: ["model"],
          message: parsed.error.issues[0].message,
        });
    }
  });
export type AnimationScheduleForm = z.infer<typeof animationScheduleSchema>;
