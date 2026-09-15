import { z } from "zod";
import { oauthProxySchema } from "./oauth-proxy-schema";

export const securitySchema = z
  .object({
    account_id: z.string(),
    source_oauth_id: z.string().optional(),
    source_checkpoint_id: z.string().optional(),
    proxy_url: oauthProxySchema.optional(),
    operation: z.enum(["password", "totp"]),
    password: z.string().max(128, "新密码最多 128 个字符"),
  })
  .superRefine((value, context) => {
    if (
      [value.account_id, value.source_oauth_id, value.source_checkpoint_id].filter(Boolean)
        .length !== 1
    )
      context.addIssue({ code: "custom", path: ["account_id"], message: "请选择一个安全设置来源" });
    if (value.operation !== "password") return;
    if (
      value.password.length < 12 ||
      !/[A-Z]/.test(value.password) ||
      !/[a-z]/.test(value.password) ||
      !/[0-9]/.test(value.password) ||
      !/[^A-Za-z0-9]/.test(value.password) ||
      /[\r\n\0]/.test(value.password)
    ) {
      context.addIssue({
        code: "custom",
        path: ["password"],
        message: "新密码需要 12～128 个字符，并包含大小写字母、数字和符号",
      });
    }
  });

export type SecurityValues = z.infer<typeof securitySchema>;

export const securityBatchSchema = z
  .object({
    account_ids: z.array(z.string().min(1)).max(500, "一次最多选择 500 个账号"),
    source_indexes: z.array(z.number().int().min(0).max(499)).max(500).optional(),
    operation: z.enum(["password", "totp"]),
    password: z.string(),
    proxy_url: oauthProxySchema.optional(),
  })
  .superRefine((value, context) => {
    if (value.account_ids.length > 0 === (value.source_indexes?.length ?? 0) > 0)
      context.addIssue({
        code: "custom",
        path: ["account_ids"],
        message: "请选择一种账号来源及至少一个账号",
      });
    const result = securitySchema.safeParse({
      account_id: value.account_ids[0] ?? "",
      operation: value.operation,
      password: value.password,
    });
    if (!result.success) {
      for (const issue of result.error.issues) {
        if (issue.path[0] === "password")
          context.addIssue({ code: "custom", path: ["password"], message: issue.message });
      }
    }
    if (new Set(value.account_ids).size !== value.account_ids.length)
      context.addIssue({ code: "custom", path: ["account_ids"], message: "账号不能重复选择" });
  });
export type SecurityBatchValues = z.infer<typeof securityBatchSchema>;
