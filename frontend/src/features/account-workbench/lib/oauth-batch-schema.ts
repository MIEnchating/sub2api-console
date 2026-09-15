import { z } from "zod";
import { maxInputBytes } from "../constants";
import { oauthProxySchema } from "./oauth-proxy-schema";
import { oauthSMSDefaults, oauthSMSFields, validateOAuthSMS } from "./oauth-sms-schema";

export const oauthBatchSchema = oauthSMSFields
  .extend({
    recovery_enabled: z.boolean(),
    proxy_url: oauthProxySchema,
    content: z
      .string()
      .trim()
      .min(1, "请填写至少一个授权账号")
      .refine(
        (value) => new TextEncoder().encode(value).byteLength <= maxInputBytes,
        "单批授权内容不能超过 2 MB",
      ),
  })
  .superRefine(validateOAuthSMS);
export type OAuthBatchValues = z.infer<typeof oauthBatchSchema>;
export const oauthBatchDefaults: OAuthBatchValues = {
  ...oauthSMSDefaults,
  content: "",
  proxy_url: "",
  recovery_enabled: false,
};
