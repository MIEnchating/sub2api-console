import { z } from "zod";
import { smsAPIKeyPattern } from "./oauth-sms-schema";

export const smsInspectSchema = z
  .object({
    provider: z.enum(["smsbower", "luban", "custom"]),
    api_key: z.string().trim().max(256),
    service_id: z.string().trim().max(80),
    custom_entries: z.string().trim().max(500000),
  })
  .superRefine((value, context) => {
    if (value.provider !== "custom" && !smsAPIKeyPattern.test(value.api_key))
      context.addIssue({ code: "custom", path: ["api_key"], message: "请填写原供应商 API Key" });
    if (value.provider === "luban" && !/^[A-Za-z0-9._:-]{1,80}$/.test(value.service_id))
      context.addIssue({ code: "custom", path: ["service_id"], message: "请填写原供应商编号" });
    if (value.provider === "custom" && !value.custom_entries)
      context.addIssue({
        code: "custom",
        path: ["custom_entries"],
        message: "请填写原自定义接码列表",
      });
  });
export type SMSInspectValues = z.infer<typeof smsInspectSchema>;
