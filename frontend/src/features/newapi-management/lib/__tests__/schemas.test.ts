import { describe, expect, it } from "vitest";

import { newAPIChannelSchema, newAPIPlatformSchema, parseModelList } from "../schemas";

describe("New API 管理表单", () => {
  it("拒绝无效平台地址和空 User ID", () => {
    const result = newAPIPlatformSchema.safeParse({
      name: "生产平台",
      base_url: "not-a-url",
      admin_key: "secret",
      user_id: "",
    });

    expect(result.success).toBe(false);
  });

  it("渠道模型按逗号和换行去重", () => {
    expect(parseModelList("gpt-5, claude-sonnet-4\ngpt-5")).toEqual(["claude-sonnet-4", "gpt-5"]);
  });

  it("拒绝缺少服务密钥的渠道", () => {
    const result = newAPIChannelSchema.safeParse({
      name: "Sub2API 标准组",
      sub2api_group_id: "6",
      base_url: "https://sub2api.example/v1",
      service_key: "",
      models: "gpt-5",
    });

    expect(result.success).toBe(false);
  });
});
