import { z } from "zod";

export { parseJsonStringMap as parseStringMap } from "@/lib/json-string-map";

export const upstreamEditSchema = z.object({
  name: z.string().trim().min(1, "请输入上游名称").max(100, "上游名称不能超过 100 个字符"),
  base_url_protocol: z.enum(["https", "http"]),
  base_url: z
    .string()
    .trim()
    .min(1, "请输入 Base URL")
    .refine((value) => !value.includes("://"), "协议请使用左侧下拉选择")
    .refine((value) => {
      try {
        return Boolean(new URL(`https://${value}`).hostname);
      } catch {
        return false;
      }
    }, "请输入有效的 Base URL"),
  account_base_url: z
    .string()
    .trim()
    .url("请输入完整的 HTTP/HTTPS 地址")
    .refine((value) => /^https?:\/\//i.test(value), "账号 Base URL 仅支持 HTTP/HTTPS"),
  upstream_type: z.string().min(2, "请选择平台类型"),
  auth_mode: z.string().min(2, "请选择鉴权方式"),
  recharge_rate: z
    .string()
    .min(1, "请输入倍率")
    .refine((value) => {
      const parsed = Number(value);
      return Number.isFinite(parsed) && parsed > 0;
    }, "倍率必须是有限正数"),
  access_token: z.string(),
  refresh_token: z.string(),
  admin_key: z.string(),
  user_id: z.string(),
  headers: z.string(),
  cookies: z.string(),
  username: z.string(),
  password: z.string(),
  save_to_vault: z.boolean(),
  entry: z.string(),
});

export type UpstreamEditValues = z.infer<typeof upstreamEditSchema>;

export function composeUpstreamBaseUrl(
  values: Pick<UpstreamEditValues, "base_url_protocol" | "base_url">,
): string {
  return `${values.base_url_protocol}://${values.base_url.trim().replace(/^\/+/, "")}`;
}

export function parseUpstreamBaseUrl(
  value: string,
): Pick<UpstreamEditValues, "base_url_protocol" | "base_url"> {
  const trimmed = value.trim();
  return {
    base_url_protocol: trimmed.toLowerCase().startsWith("http://") ? "http" : "https",
    base_url: trimmed.replace(/^https?:\/\//i, ""),
  };
}

export function upstreamConnectionPayload(
  values: Pick<UpstreamEditValues, "base_url_protocol" | "base_url" | "account_base_url">,
): { base_url: string; account_base_url: string } {
  return {
    base_url: composeUpstreamBaseUrl(values),
    account_base_url: values.account_base_url.trim(),
  };
}

export type AuthModeOption = { value: string; label: string };

export function authModesForPlatform(platform: string): AuthModeOption[] {
  if (platform === "sub2api") {
    return [
      { value: "sub2api_user_token", label: "Token + 刷新 Token" },
      { value: "sub2api_user_login", label: "密码箱登录" },
      { value: "sub2api_manual_login", label: "自定义账号密码" },
    ];
  }
  if (platform === "newapi" || platform === "oneapi") {
    return [
      { value: "newapi_admin_key", label: "Admin Key + 用户 ID" },
      { value: "newapi_user_token", label: "Token" },
      { value: "newapi_user_login", label: "密码箱登录" },
      { value: "newapi_manual_login", label: "自定义账号密码" },
    ];
  }
  return [
    { value: "bearer_token", label: "Bearer Token" },
    { value: "custom_headers", label: "自定义 Header / Cookie" },
    { value: "cookie", label: "Cookie" },
  ];
}

export function defaultAuthModeForPlatform(platform: string): string {
  const modes = authModesForPlatform(platform);
  return (
    modes.find((mode) => mode.value.endsWith("_user_login"))?.value ??
    modes[0]?.value ??
    "custom_headers"
  );
}
