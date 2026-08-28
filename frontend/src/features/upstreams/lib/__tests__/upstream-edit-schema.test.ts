import { describe, expect, it } from "vitest";

import {
  authModesForPlatform,
  composeUpstreamBaseUrl,
  parseUpstreamBaseUrl,
  parseStringMap,
  upstreamEditSchema,
} from "../upstream-edit-schema";

describe("upstream edit schema", () => {
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
      base_url_protocol: "https",
      base_url: "upstream.test",
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

  it("splits and composes Base URL protocols without changing the saved URL", () => {
    expect(parseUpstreamBaseUrl("http://upstream.test/api")).toEqual({
      base_url_protocol: "http",
      base_url: "upstream.test/api",
    });
    expect(
      composeUpstreamBaseUrl({
        base_url_protocol: "https",
        base_url: "upstream.test/api",
      }),
    ).toBe("https://upstream.test/api");
  });

  it("requires the protocol to be selected separately from the Base URL address", () => {
    const result = upstreamEditSchema.safeParse({
      name: "Example",
      base_url_protocol: "https",
      base_url: "https://upstream.test",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
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
    if (!result.success) {
      expect(result.error.flatten().fieldErrors.base_url).toContain("协议请使用左侧下拉选择");
    }
  });
});
