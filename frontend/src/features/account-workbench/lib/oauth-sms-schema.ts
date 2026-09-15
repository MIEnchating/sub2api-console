import { z } from "zod";
import type { WorkbenchOAuthSMSInput } from "@/api";

export const oauthSMSFields = z.object({
  sms_provider: z.enum(["none", "smsbower", "luban", "custom"]),
  sms_api_key: z.string().trim().max(256),
  sms_service_id: z.string().trim().max(80),
  sms_country: z.string(),
  sms_max_price: z.string(),
  sms_custom_entries: z.string().max(500000, "接码列表过长，请分批授权"),
  sms_confirmed: z.boolean(),
});
export type OAuthSMSValues = z.infer<typeof oauthSMSFields>;
export const oauthSMSDefaults: OAuthSMSValues = {
  sms_provider: "none",
  sms_api_key: "",
  sms_service_id: "",
  sms_country: "",
  sms_max_price: "",
  sms_custom_entries: "",
  sms_confirmed: false,
};
export const smsProviderOptions = [
  { value: "none", label: "人工填写" },
  { value: "smsbower", label: "SMSBower" },
  { value: "luban", label: "LubanSMS" },
  { value: "custom", label: "自定义接码" },
] as const;
export const smsAPIKeyPattern = /^[A-Za-z0-9._-]{8,256}$/;

function validCustomEntries(value: string): boolean {
  const lines = value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  if (!lines.length || lines.length > 500) return false;
  return lines.every((line) => {
    const parts = line.split("----");
    if (parts.length !== 2 || !/^\+[1-9][0-9]{6,14}$/.test(parts[0].trim())) return false;
    try {
      const url = new URL(parts[1].trim());
      return url.protocol === "https:" && !url.username && !url.password && !url.hash;
    } catch {
      return false;
    }
  });
}

export function validateOAuthSMS(value: OAuthSMSValues, context: z.RefinementCtx): void {
  if (value.sms_provider === "none") return;
  if (
    (value.sms_provider === "smsbower" || value.sms_provider === "luban") &&
    !smsAPIKeyPattern.test(value.sms_api_key)
  )
    context.addIssue({
      code: "custom",
      path: ["sms_api_key"],
      message: "请填写有效的接码供应商 API Key",
    });
  if (value.sms_provider === "luban" && !/^[A-Za-z0-9._:-]{1,80}$/.test(value.sms_service_id))
    context.addIssue({
      code: "custom",
      path: ["sms_service_id"],
      message: "请填写 LubanSMS 供应商编号",
    });
  if (value.sms_provider === "smsbower") {
    if (!/^[0-9]{1,5}$/.test(value.sms_country))
      context.addIssue({
        code: "custom",
        path: ["sms_country"],
        message: "请读取国家价格并选择可用国家",
      });
    if (
      !/^[0-9]+(?:\.[0-9]{1,64})?$/.test(value.sms_max_price) ||
      !/[1-9]/.test(value.sms_max_price)
    )
      context.addIssue({
        code: "custom",
        path: ["sms_max_price"],
        message: "缺少有效供应商价格，请重新读取国家价格",
      });
  }
  if (value.sms_provider === "custom" && !validCustomEntries(value.sms_custom_entries))
    context.addIssue({
      code: "custom",
      path: ["sms_custom_entries"],
      message: "每行填写 +国际手机号----HTTPS 接码地址，最多 500 行",
    });
  if (!value.sms_confirmed)
    context.addIssue({
      code: "custom",
      path: ["sms_confirmed"],
      message: "请确认手机号绑定与接码费用",
    });
}

export function oauthSMSInput(value: OAuthSMSValues): WorkbenchOAuthSMSInput | undefined {
  if (value.sms_provider === "none") return;
  if (value.sms_provider === "custom")
    return {
      provider: "custom",
      custom_entries: value.sms_custom_entries,
      confirmed: value.sms_confirmed,
    };
  if (value.sms_provider === "luban")
    return {
      provider: "luban",
      api_key: value.sms_api_key,
      service_id: value.sms_service_id,
      confirmed: value.sms_confirmed,
    };
  return {
    provider: "smsbower",
    api_key: value.sms_api_key,
    country: value.sms_country,
    max_price: value.sms_max_price,
    confirmed: value.sms_confirmed,
  };
}

export const oauthSMSAttachmentSchema = oauthSMSFields.superRefine((value, context) => {
  validateOAuthSMS(value, context);
  if (value.sms_provider === "none")
    context.addIssue({ code: "custom", path: ["sms_provider"], message: "请选择接码服务" });
});
