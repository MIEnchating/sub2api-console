import { z } from "zod";
import type { OnboardingRequest } from "@/api";

export function parseOnboardingProbeModels(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/\r?\n/)
    .map((model) => model.trim())
    .filter((model) => {
      if (!model || seen.has(model.toLocaleLowerCase())) return false;
      seen.add(model.toLocaleLowerCase());
      return true;
    });
}

const modelsField = z.string().superRefine((value, context) => {
  const models = parseOnboardingProbeModels(value);
  if (models.length > 20) context.addIssue({ code: "custom", message: "最多配置 20 个探活模型" });
  if (models.some((model) => [...model].length > 256))
    context.addIssue({ code: "custom", message: "每个探活模型名称最多 256 个字符" });
});

export const onboardingProbeModelsSchema = z.object({
  accounts: z.array(z.object({ id: z.string(), models: modelsField })),
});
export type OnboardingAccountProbeModels = Record<string, string[]>;

export function onboardingAccountPreviewID(request: OnboardingRequest): string {
  return JSON.stringify([
    request.host,
    request.upstream_group_id,
    request.local_group_ids ?? [request.local_group_id],
    request.account_ids ?? [],
  ]);
}

export function withOnboardingProbeModels(
  requests: OnboardingRequest[],
  models: OnboardingAccountProbeModels,
): OnboardingRequest[] {
  return requests.map((request) => {
    if ((request.account_ids?.length ?? 0) > 0) return request;
    const accountModels = models[onboardingAccountPreviewID(request)];
    if (!accountModels) throw new Error("新增账号的探活配置与预览不一致，请重新预览");
    return { ...request, test_models: [...accountModels] };
  });
}
