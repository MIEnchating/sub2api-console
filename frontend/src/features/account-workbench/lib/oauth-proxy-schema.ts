import { z } from "zod";

export const oauthProxySchema = z
  .string()
  .trim()
  .max(4096, "登录代理地址不能超过 4096 个字符")
  .refine((value) => {
    if (!value) return true;
    try {
      const url = new URL(value);
      return (
        ["http:", "https:", "socks5:"].includes(url.protocol) &&
        !!url.hostname &&
        !url.hash &&
        !url.search &&
        (url.pathname === "" || url.pathname === "/")
      );
    } catch {
      return false;
    }
  }, "请填写有效的 HTTP、HTTPS 或 SOCKS5 代理地址，不含路径、查询参数或片段");

export const oauthProxyFormSchema = z.object({ proxy_url: oauthProxySchema });
export type OAuthProxyValues = z.infer<typeof oauthProxyFormSchema>;
