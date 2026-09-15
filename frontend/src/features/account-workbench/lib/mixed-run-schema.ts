import { z } from "zod";
import { maxInputBytes } from "../constants";
import { oauthProxySchema } from "./oauth-proxy-schema";
import { oauthSMSDefaults, oauthSMSFields, validateOAuthSMS } from "./oauth-sms-schema";

export const mixedRunSchema = oauthSMSFields
  .extend({
    content: z
      .string()
      .trim()
      .min(1, "请填写 JSON、Refresh Token 或登录资料")
      .refine(
        (value) => new TextEncoder().encode(value).byteLength <= maxInputBytes,
        "混合运行内容不能超过 2 MB",
      ),
    template_id: z.string(),
    check_after_import: z.boolean(),
    model: z.string().trim().max(200, "模型名称不能超过 200 个字符"),
    export_only: z.boolean(),
    recovery_enabled: z.boolean(),
    proxy_url: oauthProxySchema,
  })
  .superRefine((value, context) => {
    validateOAuthSMS(value, context);
    if (!value.export_only && value.check_after_import && !value.model)
      context.addIssue({ code: "custom", path: ["model"], message: "启用导入后检测时请填写模型" });
  });
export type MixedRunValues = z.infer<typeof mixedRunSchema>;
export const mixedRunDefaults: MixedRunValues = {
  ...oauthSMSDefaults,
  content: "",
  template_id: "",
  check_after_import: false,
  model: "",
  export_only: false,
  recovery_enabled: false,
  proxy_url: "",
};
