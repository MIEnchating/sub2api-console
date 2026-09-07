import { describe, expect, it } from "vitest";

import { modelCheckConfigurationSchema } from "../model-check-configuration-schema";

describe("模型检测画像表单校验", () => {
  it("JSON 对象配置可以保存为草稿", () => {
    const result = modelCheckConfigurationSchema.safeParse({
      note: "更新题库",
      payload_json: '{"claude_profiles":{},"sol_profile":{}}',
    });

    expect(result.success).toBe(true);
  });

  it("JSON 无效或根节点不是对象时阻止保存", () => {
    expect(modelCheckConfigurationSchema.safeParse({ note: "", payload_json: "{" }).success).toBe(
      false,
    );
    expect(modelCheckConfigurationSchema.safeParse({ note: "", payload_json: "[]" }).success).toBe(
      false,
    );
  });
});
