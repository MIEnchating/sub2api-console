import { describe, expect, it } from "vitest";
import { oauthProxySchema } from "../lib/oauth-proxy-schema";
import { oauthLoginDefaults, oauthLoginInput, oauthLoginSchema } from "../lib/oauth-login-schema";
import { oauthBatchDefaults, oauthBatchSchema } from "../lib/oauth-batch-schema";

describe("登录代理校验", () => {
  it.each([
    "",
    "https://proxy.example.test:443",
    "http://name-{session}:secret@proxy.example.test:8080",
    "socks5://name:secret@proxy.example.test:1080",
  ])("允许合法登录代理 %s 且保留原始session占位", (proxy) => {
    expect(oauthProxySchema.parse(proxy)).toBe(proxy);
  });
  it.each([
    "file:///tmp/socket",
    "ftp://proxy.example.test",
    "https://proxy.example.test/path",
    "https://proxy.example.test?key=value",
    "https://proxy.example.test#x",
    "proxy.example.test:8080",
  ])("拒绝不支持的代理地址 %s", (proxy) => {
    expect(oauthProxySchema.safeParse(proxy).success).toBe(false);
  });
  it("自动授权提交代理但直连不补造代理字段", () => {
    const direct = oauthLoginSchema.parse({
      ...oauthLoginDefaults,
      email: "operator@example.test",
    });
    expect(oauthLoginInput(direct).login).not.toHaveProperty("proxy_url");
    const proxied = oauthLoginSchema.parse({
      ...direct,
      proxy_url: "socks5://u-{session}:pw@proxy.example.test:1080",
    });
    expect(oauthLoginInput(proxied).login?.proxy_url).toBe(proxied.proxy_url);
  });
  it("批量共享代理和单项代理使用一致的校验规则", () => {
    expect(
      oauthBatchSchema.safeParse({
        ...oauthBatchDefaults,
        content: "operator@example.test",
        proxy_url: "javascript:alert(1)",
      }).success,
    ).toBe(false);
  });
});
