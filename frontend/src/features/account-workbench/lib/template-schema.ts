import { z } from "zod";
export const templateSchema = z.object({
  source_id: z.string().min(1, "请选择来源账号"),
  name: z.string().trim().min(1, "请输入模板名称").max(100, "模板名称最多 100 字"),
});
export type TemplateValues = z.infer<typeof templateSchema>;
