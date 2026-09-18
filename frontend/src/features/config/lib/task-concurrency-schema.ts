import { z } from "zod";

const capacity = z
  .string()
  .trim()
  .regex(/^\d+$/, "请输入 1–10000 之间的整数")
  .refine((value) => Number(value) >= 1 && Number(value) <= 10000, "请输入 1–10000 之间的整数");

export const taskConcurrencySchema = z.object({
  limits: z.record(z.string(), capacity),
  queueCapacity: capacity,
  version: z.string(),
});
export type TaskConcurrencyValues = z.infer<typeof taskConcurrencySchema>;
