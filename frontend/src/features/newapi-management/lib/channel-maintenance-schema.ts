import { z } from "zod";

export const channelMaintenanceSchema = z.object({
  modelText: z
    .string()
    .trim()
    .min(1, "请输入要上架的模型名称")
    .refine(
      (value) => value.split(/[,，\n]/).every((model) => model.trim().length <= 255),
      "模型名称不能超过 255 个字符",
    ),
});
export type ChannelMaintenanceValues = z.infer<typeof channelMaintenanceSchema>;

export function parseChannelModelText(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[,，\n]/)
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  ];
}
