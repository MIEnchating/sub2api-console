import { z } from "zod";
import type { OnboardingRequest } from "@/api";
import { onboardingAccountPreviewID, onboardingProbeModelsSchema } from "./onboarding-probe-models";

const modelName = z
  .string()
  .trim()
  .min(1, "请输入模型名称")
  .superRefine((value, context) => {
    if ([...value].length > 256)
      context.addIssue({ code: "custom", message: "模型名称最多 256 个字符" });
    if (/[\s*?[\]{}^$|\\\p{Cc}]/u.test(value))
      context.addIssue({
        code: "custom",
        message: "请填写精确模型名称，不支持空白、通配符或控制字符",
      });
  });

export const modelMappingSchema = z
  .array(z.object({ source: modelName, target: modelName }))
  .max(100, "最多配置 100 条模型映射")
  .superRefine((rows, context) => {
    const seen = new Set<string>();
    rows.forEach((row, index) => {
      if (seen.has(row.source))
        context.addIssue({ code: "custom", path: [index, "source"], message: "请求模型不能重复" });
      seen.add(row.source);
    });
  });

export type OnboardingAccountModelMappings = Record<string, Record<string, string>>;

export function withOnboardingModelMappings(
  requests: OnboardingRequest[],
  mappings: OnboardingAccountModelMappings,
): OnboardingRequest[] {
  return requests.map((request) => {
    if (request.account_ids?.length) return request;
    const mapping = mappings[onboardingAccountPreviewID(request)];
    if (!mapping) throw new Error("新增账号的模型映射与预览不一致，请重新预览");
    if (!Object.keys(mapping).length) return request;
    return { ...request, model_mapping: { ...mapping } };
  });
}

export const onboardingConfirmationSchema = z.object({
  accounts: z.array(
    onboardingProbeModelsSchema.shape.accounts.element.extend({ mapping: modelMappingSchema }),
  ),
});
export type OnboardingConfirmationForm = z.infer<typeof onboardingConfirmationSchema>;
