import { expect, it } from "vitest";
import { accountCreationSettingsSchema } from "../account-creation-settings-schema";
import { platformProbeModelsSchema } from "../platform-probe-models-schema";

it("负载因子超过后端的 128 字符上限时阻止提交", () => {
  const result = accountCreationSettingsSchema.safeParse({
    models: "model-a",
    concurrency: "10",
    loadFactor: `1.${"0".repeat(127)}`,
    priority: "1",
    poolMode: false,
    retryCount: "3",
    retryStatusCodes: "429",
  });
  expect(result.success).toBe(false);
});

it("默认探活模型包含 256 个 Unicode 字符时与后端一致允许提交", () => {
  const result = platformProbeModelsSchema.shape.openai.safeParse("𠮷".repeat(256));
  expect(result.success).toBe(true);
  expect(platformProbeModelsSchema.shape.openai.safeParse("𠮷".repeat(257)).success).toBe(false);
});
