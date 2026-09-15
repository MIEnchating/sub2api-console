import { z } from "zod";
import { maxInputBytes } from "../constants";
import { oauthProxySchema } from "./oauth-proxy-schema";

export const mixedRunSchema = z
  .object({
    content: z
      .string()
      .trim()
      .min(1, "请填写 JSON、Refresh Token 或登录资料")
      .refine(
        (value) => new TextEncoder().encode(value).byteLength <= maxInputBytes,
        "账号批次内容不能超过 2 MB",
      ),
    template_id: z.string(),
    model: z.string().trim(),
    export_only: z.boolean(),
    proxy_url: oauthProxySchema,
  })
  .superRefine((value, context) => {
    if (!value.export_only && value.model.length > 200)
      context.addIssue({ code: "custom", path: ["model"], message: "模型名称不能超过 200 个字符" });
  });
export type MixedRunValues = z.infer<typeof mixedRunSchema>;
export const mixedRunDefaults: MixedRunValues = {
  content: "",
  template_id: "",
  model: "gpt-5.6-sol",
  export_only: false,
  proxy_url: "",
};
