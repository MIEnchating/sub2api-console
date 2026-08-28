import { describe, expect, it } from "vitest";

import { configuredSecretPlaceholder, sensitiveFieldPlaceholder } from "../sensitive-field";

describe("sensitive field display", () => {
  it("uses one redacted placeholder for every configured value", () => {
    expect(sensitiveFieldPlaceholder(true, "输入密钥")).toBe(configuredSecretPlaceholder);
    expect(configuredSecretPlaceholder).toBe("已配置，留空则不修改");
  });

  it("uses the field-specific prompt before a value is configured", () => {
    expect(sensitiveFieldPlaceholder(false, "输入密钥")).toBe("输入密钥");
  });
});
