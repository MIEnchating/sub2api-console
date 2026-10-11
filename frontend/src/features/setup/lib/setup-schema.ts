import { z } from "zod";
import {
  consolePasswordSchema,
  consoleUsernameSchema,
  credentialLength,
} from "@/features/auth/lib/credential-schema";

function validManagementURL(value: string): boolean {
  if (!/^https?:\/\//.test(value) || /[?#]/.test(value)) return false;
  try {
    const url = new URL(value);
    return (
      ["http:", "https:"].includes(url.protocol) &&
      !!url.hostname &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash
    );
  } catch {
    return false;
  }
}

export function createSetupSchema(setupTokenRequired: boolean, targetConfigured: boolean) {
  return z
    .object({
      local_export_only: z.boolean(),
      username: consoleUsernameSchema,
      password: consolePasswordSchema(),
      confirm_password: z.string().min(1, "请再次输入密码"),
      admin_base_url: z.string().trim(),
      admin_key: z.string().trim(),
      setup_token: setupTokenRequired ? z.string().trim().min(1, "请输入初始化令牌") : z.string(),
    })
    .superRefine((value, context) => {
      if (value.password !== value.confirm_password)
        context.addIssue({
          code: "custom",
          path: ["confirm_password"],
          message: "两次输入的密码不一致",
        });
      if (targetConfigured || value.local_export_only) return;
      if (!value.admin_base_url && !value.admin_key) {
        context.addIssue({
          code: "custom",
          path: ["admin_base_url"],
          message: "请填写管理地址和管理密钥，或选择仅使用本地账号工作台",
        });
        return;
      }
      if (!validManagementURL(value.admin_base_url))
        context.addIssue({
          code: "custom",
          path: ["admin_base_url"],
          message: "请输入完整的 http/https 地址",
        });
      if (!value.admin_key || credentialLength(value.admin_key) > 4096)
        context.addIssue({
          code: "custom",
          path: ["admin_key"],
          message: "Admin Key 需为 1–4096 个字符",
        });
    });
}

export type SetupForm = z.infer<ReturnType<typeof createSetupSchema>>;
