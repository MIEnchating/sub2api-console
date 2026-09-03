import { describe, expect, it } from "vitest";

import {
  newAPIChannelKeySchema,
  newAPIChannelSchema,
  newAPIPlatformSchema,
  parseModelList,
} from "../schemas";

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

  it("密码箱模式必须选择账号条目", () => {
    const result = newAPIChannelKeySchema.safeParse({
      credential_source: "vault",
      vault_entry: "",
      username: "",
      password: "",
      sub2api_group_id: "6",
    });

    expect(result.success).toBe(false);
  });

  it("自定义模式必须提供有效邮箱和密码", () => {
    const missing = newAPIChannelKeySchema.safeParse({
      credential_source: "custom",
      vault_entry: "",
      username: "not-an-email",
      password: "",
      sub2api_group_id: "6",
    });
    const valid = newAPIChannelKeySchema.safeParse({
      credential_source: "custom",
      vault_entry: "",
      username: "operator@example.test",
      password: "password",
      sub2api_group_id: "6",
    });

    expect(missing.success).toBe(false);
    expect(valid.success).toBe(true);
  });

  it("拒绝缺少密钥 ID 的渠道", () => {
    const result = newAPIChannelSchema.safeParse({
      sub2api_group_id: "6",
      key_id: "",
      models: ["gpt-5"],
      newapi_groups: ["default"],
    });

    expect(result.success).toBe(false);
  });

  it("拒绝未选择上游模型或 New API 分组的渠道", () => {
    const result = newAPIChannelSchema.safeParse({
      sub2api_group_id: "6",
      key_id: "key-7",
      base_url: "https://api.example",
      models: [],
      newapi_groups: [],
    });

    expect(result.success).toBe(false);
  });

  it("拒绝无效的渠道 API 地址", () => {
    const result = newAPIChannelSchema.safeParse({
      sub2api_group_id: "6",
      key_id: "key-7",
      base_url: "not-a-url",
      models: ["gpt-5"],
      newapi_groups: ["default"],
    });

    expect(result.success).toBe(false);
  });
});
