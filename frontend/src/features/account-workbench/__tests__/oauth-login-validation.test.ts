import { describe, expect, it } from "vitest";
import { oauthLoginDefaults, oauthLoginInput, oauthLoginSchema } from "../lib/oauth-login-schema";

describe("自动授权登录校验", () => {
  it("过短 TOTP 密钥不进入授权请求", () => {
    const result = oauthLoginSchema.safeParse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      totp_secret: "JBSWY3DP",
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error("无效登录配置不应通过校验");
    expect(result.error.issues.map((issue) => issue.path[0])).toContain("totp_secret");
  });
  it("不同 Microsoft 收信邮箱不进入授权请求", () => {
    const result = oauthLoginSchema.safeParse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      mailbox_kind: "microsoft",
      mailbox_email: "other@example.test",
      client_id: "fixture",
      refresh_token: "fixture",
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error("无效收信邮箱不应通过校验");
    expect(result.error.issues.map((issue) => issue.path[0])).toContain("mailbox_email");
  });
  it("普通登录仅发送已填写的字段并规范化 TOTP 密钥", () => {
    const values = oauthLoginSchema.parse({
      ...oauthLoginDefaults,
      email: " operator@example.test ",
      password: " password with spaces ",
      totp_secret: "jbsw y3dp ehpk 3pxp",
      workspace_id: "workspace-42",
    });
    expect(oauthLoginInput(values)).toEqual({
      login: {
        email: "operator@example.test",
        password: " password with spaces ",
        totp_secret: "JBSWY3DPEHPK3PXP",
        workspace_id: "workspace-42",
      },
    });
  });

  it("HTTP 邮箱拒绝带身份信息地址和非字符串请求头", () => {
    const result = oauthLoginSchema.safeParse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      mailbox_kind: "http",
      url: "https://operator:secret@mail.example.test",
      headers: '{"Authorization":42}',
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error("无效邮箱配置不应通过校验");
    expect(result.error.issues.map((issue) => issue.path[0])).toEqual(
      expect.arrayContaining(["url", "headers"]),
    );
  });

  it("HTTP POST 邮箱保留 JSON 原文且 GET 不发送请求体", () => {
    const values = oauthLoginSchema.parse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      mailbox_kind: "http",
      method: "POST",
      url: "https://mail.example.test/inbox",
      headers: '{"X-Mailbox-Key":"fixture-key"}',
      body: '{ "mailbox": 9007199254740993 }',
    });
    expect(oauthLoginInput(values).login?.mailbox).toEqual({
      kind: "http",
      method: "POST",
      url: "https://mail.example.test/inbox",
      headers: { "X-Mailbox-Key": "fixture-key" },
      body: '{ "mailbox": 9007199254740993 }',
    });
    expect(oauthLoginInput({ ...values, method: "GET" }).login?.mailbox).not.toHaveProperty("body");
  });

  it("Microsoft 邮箱需要客户端与令牌，缺省收信邮箱使用登录邮箱", () => {
    expect(
      oauthLoginSchema.safeParse({
        ...oauthLoginDefaults,
        email: "operator@example.test",
        mailbox_kind: "microsoft",
      }).success,
    ).toBe(false);
    const values = oauthLoginSchema.parse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      mailbox_kind: "microsoft",
      client_id: "client-fixture",
      refresh_token: "mailbox-fixture-rt",
    });
    expect(oauthLoginInput(values).login?.mailbox).toEqual({
      kind: "microsoft",
      email: "operator@example.test",
      client_id: "client-fixture",
      refresh_token: "mailbox-fixture-rt",
    });
  });

  it("HTTP POST 请求体无效时把错误定位到请求体字段", () => {
    const result = oauthLoginSchema.safeParse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      mailbox_kind: "http",
      method: "POST",
      url: "https://mail.example.test/inbox",
      body: "not json",
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error("无效 JSON 不应通过校验");
    expect(result.error.issues).toEqual(
      expect.arrayContaining([expect.objectContaining({ path: ["body"] })]),
    );
  });
});
