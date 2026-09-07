import { describe, expect, it } from "vitest";

import { authMethodSummary } from "../upstream-auth-labels";
import { authMethodLabel } from "../upstream-edit-schema";

describe("上游鉴权方式摘要", () => {
  it.each(["refresh_token", "refresh"])("通过 %s 恢复时只显示一次刷新 Token", (recoveryMethod) => {
    expect(authMethodSummary("sub2api_user_token", recoveryMethod)).toBe(
      "Token · 恢复：刷新 Token",
    );
  });

  it("密码箱登录后刷新 Token 时区分鉴权方式与恢复方式", () => {
    expect(authMethodSummary("sub2api_user_login", "refresh_token")).toBe(
      "密码箱登录 · 恢复：刷新 Token",
    );
  });

  it("通过密码箱恢复时展示恢复来源", () => {
    expect(authMethodSummary("newapi_user_login", "vault")).toBe("密码箱登录 · 恢复：密码箱");
  });

  it.each([null, undefined, ""])("恢复方式为 %s 时不显示恢复后缀", (recoveryMethod) => {
    expect(authMethodSummary("sub2api_user_token", recoveryMethod)).toBe("Token");
  });

  it("没有鉴权记录时显示尚未记录", () => {
    expect(authMethodSummary(null, null)).toBe("尚未记录");
  });

  it("未识别的方式保留服务端值", () => {
    expect(authMethodSummary("new_method", "new_recovery")).toBe("new_method · 恢复：new_recovery");
  });

  it("配置凭据时仍显示完整的 Token 与刷新 Token 名称", () => {
    expect(authMethodLabel("sub2api_user_token")).toBe("Token + 刷新 Token");
  });
});
