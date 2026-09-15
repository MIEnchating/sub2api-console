import { describe, expect, it } from "vitest";
import { securitySchema } from "../lib/security-schema";

describe("安全设置参数", () => {
  it("指定授权来源时无需托管账号 ID", () => {
    expect(
      securitySchema.safeParse({
        account_id: "",
        source_oauth_id: "oauth-source",
        operation: "totp",
        password: "",
      }).success,
    ).toBe(true);
  });
  it("同时指定托管账号和授权来源时拒绝提交", () => {
    expect(
      securitySchema.safeParse({
        account_id: "42",
        source_oauth_id: "oauth-source",
        operation: "totp",
        password: "",
      }).success,
    ).toBe(false);
  });
  it("提供非代理协议时拒绝安全任务输入", () => {
    expect(
      securitySchema.safeParse({
        account_id: "42",
        operation: "totp",
        password: "",
        proxy_url: "file:///etc/passwd",
      }).success,
    ).toBe(false);
  });
  it("设置密码时拒绝缺少必要复杂度的新密码", () => {
    expect(
      securitySchema.safeParse({
        account_id: "42",
        operation: "password",
        password: "onlylowercase",
      }).success,
    ).toBe(false);
  });
  it("设置密码时允许包含大小写数字符号的有效密码", () => {
    expect(
      securitySchema.safeParse({
        account_id: "42",
        operation: "password",
        password: "StrongPassword-2026!",
      }).success,
    ).toBe(true);
  });
  it("启用双重验证时不要求新密码", () => {
    expect(
      securitySchema.safeParse({ account_id: "42", operation: "totp", password: "" }).success,
    ).toBe(true);
  });
  it("未选择账号时拒绝提交", () => {
    expect(
      securitySchema.safeParse({ account_id: "", operation: "totp", password: "" }).success,
    ).toBe(false);
  });
});
