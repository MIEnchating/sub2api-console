import { z } from "zod";

export const automationTaskGroupSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "请输入任务组名称")
    .refine((name) => [...name].length <= 128, "任务组名称不能超过 128 个字符"),
  schedule_ids: z
    .array(z.string().regex(/^[a-f0-9]{32}$/))
    .min(2, "请至少选择两个运营任务")
    .max(50, "一个任务组最多包含 50 个运营任务"),
});

export type AutomationTaskGroupForm = z.infer<typeof automationTaskGroupSchema>;
