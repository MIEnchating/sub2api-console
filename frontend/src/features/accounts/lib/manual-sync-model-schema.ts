import { z } from "zod";

export const manualSyncModelSchema = z.object({
  model: z
    .string()
    .trim()
    .min(1, "请输入模型名称")
    .max(256, "模型名称不能超过 256 个字符")
    .refine(
      (value) =>
        !/[\s*?]/.test(value) &&
        [...value].every((char) => char.charCodeAt(0) >= 32 && char.charCodeAt(0) !== 127),
      "请输入不含空白和通配符的具体模型名称",
    ),
});
export type ManualSyncModelFormValues = z.infer<typeof manualSyncModelSchema>;
