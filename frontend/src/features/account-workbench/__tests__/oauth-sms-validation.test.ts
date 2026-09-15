import { describe, expect, it } from "vitest";
import { oauthLoginDefaults, oauthLoginInput, oauthLoginSchema } from "../lib/oauth-login-schema";

describe("授权短信配置校验", () => {
  it("启用自定义接码但未确认手机号绑定时禁止启动", () => {
    const result = oauthLoginSchema.safeParse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      sms_provider: "custom",
      sms_custom_entries: "+12025550142----https://sms.example.test/code",
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error("未确认绑定不应通过校验");
    expect(result.error.issues.map((issue) => issue.path[0])).toContain("sms_confirmed");
  });
  it("LubanSMS 配置使用供应商编号且不发送其他模式字段", () => {
    const value = oauthLoginSchema.parse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      sms_provider: "luban",
      sms_api_key: "fixture-key",
      sms_service_id: "supplier-42",
      sms_country: "15",
      sms_max_price: "0.01",
      sms_confirmed: true,
    });
    expect(oauthLoginInput(value).login?.sms).toEqual({
      provider: "luban",
      api_key: "fixture-key",
      service_id: "supplier-42",
      confirmed: true,
    });
  });
  it("SMSBower 保留供应商价格字符串的原始精度", () => {
    const price = "0.000000000000000000001";
    const value = oauthLoginSchema.parse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      sms_provider: "smsbower",
      sms_api_key: "fixture-key",
      sms_country: "15",
      sms_max_price: price,
      sms_confirmed: true,
    });
    expect(oauthLoginInput(value).login?.sms?.max_price).toBe(price);
  });
  it("自定义接码拒绝非国际号码或非 HTTPS 地址", () => {
    const result = oauthLoginSchema.safeParse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
      sms_provider: "custom",
      sms_custom_entries: "123----http://sms.example.test/code",
      sms_confirmed: true,
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error("无效接码条目不应通过校验");
    expect(result.error.issues.map((issue) => issue.path[0])).toContain("sms_custom_entries");
  });
});
