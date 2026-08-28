import { z } from "zod";

export const modelCheckSchema = z
  .object({
    account_ids: z
      .array(z.string().regex(/^[1-9]\d*$/))
      .min(1, "请选择至少一个账号")
      .max(20),
    models: z.array(z.string().trim().min(1).max(256)).min(1, "请选择至少一个模型").max(20),
    rounds: z.number().int().min(1, "检测轮次不能小于 1").max(3, "检测轮次不能超过 3"),
    timeout_seconds: z.number().int().min(5, "超时不能小于 5 秒").max(120, "超时不能超过 120 秒"),
  })
  .superRefine((value, context) => {
    if (value.account_ids.length * value.models.length > 100) {
      context.addIssue({
        code: "custom",
        path: ["models"],
        message: "单次检测不能超过 100 个账号模型组合",
      });
    }
  });

export type ModelCheckForm = z.infer<typeof modelCheckSchema>;
