import { z } from "zod";

const probeModel = z.string().trim().max(256, "模型名称不能超过 256 个字符");

export const platformProbeModelsSchema = z.object({
  openai: probeModel,
  anthropic: probeModel,
  gemini: probeModel,
  antigravity: probeModel,
  grok: probeModel,
  kimi: probeModel,
  zhipu: probeModel,
  deepseek: probeModel,
  composite: probeModel,
});

export type PlatformProbeModelsValues = z.infer<typeof platformProbeModelsSchema>;
