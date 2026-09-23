import { z } from "zod";

const modelID = z
  .string()
  .trim()
  .min(1, "请输入模型 ID")
  .max(256, "模型 ID 不能超过 256 个字符")
  .refine((value) => !/\p{Cc}/u.test(value), "模型 ID 不能包含控制字符");
export const animationSchema = z.object({
  account_ids: z.array(z.string().regex(/^[1-9]\d*$/)).min(1, "请选择至少一个账号"),
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
    precheck_questions: z.array(z.enum(["candy"])).optional(),
    mode: z.enum(["animation", "precheck", "both"]).optional(),
    schedule_type: z.enum(["interval", "daily"]).optional(),
    daily_time: z.string().optional(),
    daily_times: z.array(z.string()).optional(),
    timezone: z.literal("Asia/Shanghai").optional(),
    account_id: z.string().regex(/^[1-9]\d*$/),
    enabled: z.boolean(),
    model: z.string(),
    interval_minutes: z.number().int(),
    timeout_seconds: z.number().int().min(5, "超时不能小于 5 秒").max(120, "超时不能超过 120 秒"),
    version: z.number().int().min(0),
  })
  .superRefine((value, context) => {
    if (
      value.schedule_type !== "daily" &&
      (value.interval_minutes < 1 || value.interval_minutes > 1440)
    ) {
      context.addIssue({
        code: "custom",
        path: ["interval_minutes"],
        message: "检测间隔必须在 1 到 1440 分钟之间",
      });
    }
    if (value.schedule_type === "daily") {
      const times = value.daily_times ?? (value.daily_time ? [value.daily_time] : []);
      if (
        times.length < 1 ||
        times.length > 24 ||
        times.some((time) => !/^([01]\d|2[0-3]):[0-5]\d$/.test(time))
      ) {
        context.addIssue({
          code: "custom",
          path: ["daily_times"],
          message: "请选择每天检测的时间",
        });
      } else if (new Set(times).size !== times.length) {
        context.addIssue({ code: "custom", path: ["daily_times"], message: "检测时间不能重复" });
      }
    }
    if (
      (value.mode === "precheck" || value.mode === "both") &&
      value.precheck_questions !== undefined &&
      (value.precheck_questions.length === 0 ||
        new Set(value.precheck_questions).size !== value.precheck_questions.length)
    ) {
      context.addIssue({
        code: "custom",
        path: ["precheck_questions"],
        message: "请选择至少一道不重复的前置检测题目",
      });
    }
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
