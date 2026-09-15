import { z } from "zod";
import type { WorkbenchOAuthStartInput } from "@/api";
import { oauthProxySchema } from "./oauth-proxy-schema";
import {
  oauthSMSDefaults,
  oauthSMSFields,
  oauthSMSInput,
  validateOAuthSMS,
} from "./oauth-sms-schema";

const headersSchema = z.record(z.string().min(1), z.string());
function parseHeaders(value: string): Record<string, string> | null {
  try {
    const parsed: unknown = JSON.parse(value);
    const result = headersSchema.safeParse(parsed);
    return result.success ? result.data : null;
  } catch {
    return null;
  }
}

export const oauthLoginSchema = z
  .object({
    email: z.string().trim().email("请填写有效的登录邮箱"),
    proxy_url: oauthProxySchema,
    password: z.string().max(2048, "密码不能超过 2048 个字符"),
    totp_secret: z
      .string()
      .trim()
      .max(200)
      .refine((value) => {
        if (!value) return true;
        const normalized = value.replaceAll(" ", "");
        return (
          normalized.length >= 16 && normalized.length <= 128 && /^[A-Z2-7]+=*$/i.test(normalized)
        );
      }, "请填写有效的 TOTP 密钥"),
    workspace_id: z.string().trim().max(200, "工作区 ID 不能超过 200 个字符"),
    mailbox_kind: z.enum(["none", "http", "microsoft"]),
    url: z.string().trim(),
    method: z.enum(["GET", "POST"]),
    headers: z.string().max(16384, "请求头不能超过 16 KB"),
    body: z.string().max(65536, "请求体不能超过 64 KB"),
    mailbox_email: z.string().trim(),
    client_id: z.string().trim().max(200),
    refresh_token: z.string().trim().max(16384),
    ...oauthSMSFields.shape,
  })
  .superRefine((value, context) => {
    validateOAuthSMS(value, context);
    if (value.mailbox_kind === "http") {
      let validURL = false;
      try {
        const url = new URL(value.url);
        validURL = url.protocol === "https:" && !url.username && !url.password && !url.hash;
      } catch {
        /* Field error below includes the expected URL shape. */
      }
      if (!validURL)
        context.addIssue({
          code: "custom",
          path: ["url"],
          message: "请填写不含用户名、密码或片段的 HTTPS 邮箱地址",
        });
      if (parseHeaders(value.headers) === null)
        context.addIssue({
          code: "custom",
          path: ["headers"],
          message: "请求头必须是 JSON 对象，且所有值均为字符串",
        });
      if (value.method === "POST" && value.body.trim()) {
        try {
          JSON.parse(value.body);
        } catch {
          context.addIssue({ code: "custom", path: ["body"], message: "请求体必须是有效 JSON" });
        }
      }
    }
    if (value.mailbox_kind === "microsoft") {
      if (value.mailbox_email && !z.email().safeParse(value.mailbox_email).success)
        context.addIssue({
          code: "custom",
          path: ["mailbox_email"],
          message: "请填写有效的收信邮箱",
        });
      if (value.mailbox_email && value.mailbox_email.toLowerCase() !== value.email.toLowerCase())
        context.addIssue({
          code: "custom",
          path: ["mailbox_email"],
          message: "Microsoft 收信邮箱必须与登录邮箱一致",
        });
      if (!value.client_id)
        context.addIssue({
          code: "custom",
          path: ["client_id"],
          message: "请填写 Microsoft 客户端 ID",
        });
      if (!value.refresh_token)
        context.addIssue({
          code: "custom",
          path: ["refresh_token"],
          message: "请填写 Microsoft Refresh Token",
        });
    }
  });
export type OAuthLoginValues = z.infer<typeof oauthLoginSchema>;

export const oauthLoginDefaults: OAuthLoginValues = {
  email: "",
  proxy_url: "",
  password: "",
  totp_secret: "",
  workspace_id: "",
  mailbox_kind: "none",
  url: "",
  method: "GET",
  headers: "{}",
  body: "",
  mailbox_email: "",
  client_id: "",
  refresh_token: "",
  ...oauthSMSDefaults,
};

export function oauthLoginInput(values: OAuthLoginValues): WorkbenchOAuthStartInput {
  const login: NonNullable<WorkbenchOAuthStartInput["login"]> = { email: values.email };
  if (values.proxy_url) login.proxy_url = values.proxy_url;
  if (values.password) login.password = values.password;
  if (values.totp_secret) login.totp_secret = values.totp_secret.replaceAll(" ", "").toUpperCase();
  if (values.workspace_id) login.workspace_id = values.workspace_id;
  if (values.mailbox_kind === "http") {
    login.mailbox = {
      kind: "http",
      url: values.url,
      method: values.method,
      headers: parseHeaders(values.headers) ?? {},
    };
    if (values.method === "POST" && values.body.trim()) login.mailbox.body = values.body;
  }
  if (values.mailbox_kind === "microsoft") {
    login.mailbox = {
      kind: "microsoft",
      email: values.mailbox_email || values.email,
      client_id: values.client_id,
      refresh_token: values.refresh_token,
    };
  }
  const sms = oauthSMSInput(values);
  if (sms) login.sms = sms;
  return { login };
}
