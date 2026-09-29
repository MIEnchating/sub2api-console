import { z } from "zod";

export const accountGroupsSchema = z.object({
  groupIds: z
    .array(z.string().regex(/^[1-9]\d*$/, "请选择有效分组"))
    .min(1, "请至少选择一个目标分组")
    .max(50, "最多选择 50 个目标分组")
    .refine((ids) => new Set(ids).size === ids.length, "目标分组不能重复"),
});
export type AccountGroupsForm = z.infer<typeof accountGroupsSchema>;
