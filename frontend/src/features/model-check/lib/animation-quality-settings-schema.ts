import { z } from "zod";
import { modelID } from "./animation-schema";
export const qualitySettingsSchema = z
  .object({
    enabled: z.boolean(),
    account_id: z.string(),
    model: z.union([z.literal(""), modelID]),
    timeout_seconds: z.number().int().min(5, "超时至少为 5 秒").max(120, "超时最多为 120 秒"),
    version: z.number().int().nonnegative(),
  })
  .superRefine((value, ctx) => {
    if (!value.enabled) return;
    if (!value.account_id)
      ctx.addIssue({ code: "custom", path: ["account_id"], message: "请选择评审账号" });
    if (!value.model.trim())
      ctx.addIssue({ code: "custom", path: ["model"], message: "请输入评审模型" });
  });
export type QualitySettingsForm = z.infer<typeof qualitySettingsSchema>;
