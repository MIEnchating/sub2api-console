import { describe, expect, it } from "vitest";

import {
  authModesForPlatform,
  defaultAuthModeForPlatform,
  parseStringMap,
  upstreamEditSchema,
  upstreamConnectionPayload,
} from "../upstream-edit-schema";

describe("upstream edit schema", () => {
  it("defaults supported upstreams to password-vault login", () => {
    expect(defaultAuthModeForPlatform("sub2api")).toBe("sub2api_user_login");
    expect(defaultAuthModeForPlatform("newapi")).toBe("newapi_user_login");
    expect(defaultAuthModeForPlatform("custom")).toBe("bearer_token");
  });

  it("uses platform-specific authentication choices", () => {
    expect(authModesForPlatform("sub2api").map((item) => item.value)).toEqual([
      "sub2api_user_token",
      "sub2api_user_login",
      "sub2api_manual_login",
    ]);
    expect(authModesForPlatform("newapi").map((item) => item.value)).toEqual([
      "newapi_admin_key",
      "newapi_user_token",
      "newapi_user_login",
      "newapi_manual_login",
    ]);
  });

  it.each(["0", "-1", "NaN", "Infinity", "bad"])("rejects invalid mapping %s", (rechargeRate) => {
    const result = upstreamEditSchema.safeParse({
      name: "Example",
      base_url: "https://upstream.test",
      account_base_url: "https://account-api.test/v1",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: rechargeRate,
      access_token: "",
      refresh_token: "",
      admin_key: "",
      user_id: "",
      headers: "",
      cookies: "",
      username: "",
      password: "",
      save_to_vault: false,
      entry: "",
    });

    expect(result.success).toBe(false);
  });

  it("accepts string-only custom Header maps", () => {
    expect(parseStringMap('{"Authorization":"Bearer secret"}', "Headers")).toEqual({
      Authorization: "Bearer secret",
    });
    expect(() => parseStringMap('{"X-User":9}', "Headers")).toThrow("值必须是字符串");
  });

  it("填写完整上游地址时无需另填 Host，账号地址独立保存", () => {
    expect(
      upstreamConnectionPayload({
        base_url: " https://upstream.test:8443/admin ",
        account_base_url: "https://account-api.test/v1",
      }),
    ).toEqual({
      base_url: "https://upstream.test:8443/admin",
      account_base_url: "https://account-api.test/v1",
    });
  });

  const connectionSchema = upstreamEditSchema.pick({ base_url: true });

  it.each(["https://upstream.test", "http://upstream.test:8080/admin", "https://[::1]:8443"])(
    "填写完整上游地址 %s 时通过校验",
    (baseURL) => {
      expect(connectionSchema.safeParse({ base_url: baseURL }).success).toBe(true);
    },
  );

  it.each([
    "",
    "upstream.test",
    "ftp://upstream.test",
    "https://",
    "https://user:pass@upstream.test",
    "https://upstream.test?key=value",
    "https://upstream.test#fragment",
  ])("上游地址 %s 无效或含凭据、查询、片段时拒绝保存", (baseURL) => {
    const result = connectionSchema.safeParse({ base_url: baseURL });
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error.issues[0].path).toEqual(["base_url"]);
  });
});
