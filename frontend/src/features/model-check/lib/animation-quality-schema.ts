import { z } from "zod";

export const animationQualityFormSchema = z.object({
  label: z.enum(["normal", "suspicious", "unknown"]),
  reason: z.string().trim().min(1, "请填写可复核的判断理由").max(2000, "判断理由最多 2000 字"),
  source_group: z.string().trim().max(160, "样本分组最多 160 字"),
});
export type AnimationQualityForm = z.infer<typeof animationQualityFormSchema>;
