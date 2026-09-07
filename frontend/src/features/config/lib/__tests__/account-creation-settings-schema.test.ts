import { describe, expect, it } from "vitest";

import {
  accountCreationSettingsSchema,
  parseAccountModels,
  parseRetryStatusCodes,
} from "../account-creation-settings-schema";

describe("账号创建设置参数", () => {
  it("规范化重复模型和重试状态码", () => {
    expect(parseAccountModels("gpt-5.2\ngpt-5.1-codex, gpt-5.2")).toEqual([
      "gpt-5.1-codex",
      "gpt-5.2",
    ]);
    expect(parseRetryStatusCodes("503, 429 503")).toEqual([429, 503]);
  });

  it("拒绝超出边界的负载、重试次数和状态码", () => {
    const result = accountCreationSettingsSchema.safeParse({
      models: "gpt-5.2",
      concurrency: "10",
      loadFactor: "0.5",
      priority: "1",
      poolMode: true,
      retryCount: "11",
      retryStatusCodes: "99, 600",
    });

    expect(result.success).toBe(false);
  });
});
