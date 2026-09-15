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

export const customAnimationSchema = z
  .object({
    base_url: z
      .string()
      .trim()
      .min(1, "请输入 Base URL")
      .max(2048, "Base URL 不能超过 2048 个字符")
      .refine((value) => {
        try {
          const url = new URL(value);
          return (
            /^https?:\/\//.test(value) &&
            ["http:", "https:"].includes(url.protocol) &&
            !!url.hostname &&
            !url.username &&
            !url.password &&
            !value.includes("?") &&
            !value.includes("#")
          );
        } catch {
          return false;
        }
      }, "请输入完整的 http 或 https 地址，不能包含凭据、查询参数或片段"),
    api_key: z
      .string()
      .trim()
      .min(1, "请输入 API Key")
      .max(4096, "API Key 不能超过 4096 个字符")
      .regex(/^[\x21-\x7e]+$/, "API Key 不能包含空白或非 ASCII 字符"),
    platform: z.enum(["openai", "anthropic"]),
    model: modelID,
    timeout_seconds: animationSchema.shape.timeout_seconds,
  })
  .refine((value) => !value.api_key || !value.model.includes(value.api_key), {
    path: ["model"],
    message: "模型 ID 不能包含 API Key，请检查填写内容",
  });
export type CustomAnimationForm = z.infer<typeof customAnimationSchema>;

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
