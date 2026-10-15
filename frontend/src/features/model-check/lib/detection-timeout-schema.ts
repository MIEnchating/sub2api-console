import { z } from "zod";
export const detectionTimeoutSchema = z.object({
  timeout_seconds: z
    .number()
    .int("请输入整数秒")
    .min(5, "超时不能小于 5 秒")
    .max(120, "超时不能超过 120 秒"),
});
export type DetectionTimeoutForm = z.infer<typeof detectionTimeoutSchema>;
