import { z } from "zod";
import { maxInputBytes } from "../constants";

const decimal = z
  .string()
  .trim()
  .regex(/^\d+(?:\.\d+)?$/, "请输入非负十进制数值");
export const importSchema = z
  .object({
    content: z
      .string()
      .trim()
      .min(1, "请粘贴账号 JSON 或 Refresh Token")
      .refine(
        (value) => new TextEncoder().encode(value).byteLength <= maxInputBytes,
        "输入内容不能超过 2 MB，请分批导入",
      ),
    template_id: z.string(),
    check_after_import: z.boolean(),
    model: z.string().trim().max(200, "模型名称不能超过 200 个字符"),
  })
  .refine((value) => !value.check_after_import || !!value.model, {
    path: ["model"],
    message: "启用导入后检测时请填写模型",
  });
export type ImportValues = z.infer<typeof importSchema>;
export const oauthPreviewSchema = z
  .object({
    template_id: z.string(),
    check_after_import: z.boolean(),
    model: z.string().trim().max(200, "模型名称不能超过 200 个字符"),
  })
  .refine((value) => !value.check_after_import || !!value.model, {
    path: ["model"],
    message: "启用导入后检测时请填写模型",
  });
export type OAuthPreviewValues = z.infer<typeof oauthPreviewSchema>;
export const templateSchema = z.object({
  preferred: z.boolean(),
  name: z.string().trim().min(1, "请输入模板名称").max(100, "模板名称不能超过 100 个字符"),
  priority: z.number().int().min(0, "匹配优先级不能小于 0"),
  match: z.object({
    plan_type: z.string().trim().max(100),
    email_domain: z
      .string()
      .trim()
      .max(253)
      .refine(
        (value) => !value || /^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$/i.test(value),
        "请填写邮箱域名，例如 example.com",
      ),
  }),
  config: z.object({
    concurrency: z.number().int().min(1, "并发数至少为 1").max(1000, "并发数最多为 1000"),
    priority: z.number().int().min(0, "账号优先级不能小于 0"),
    rate_multiplier: decimal,
    group_ids: z.array(z.string().min(1)),
    auto_pause_on_expired: z.boolean(),
    notes: z.string().max(2000).optional(),
    proxy_id: z.string().nullable().optional(),
    load_factor: decimal.nullable().optional(),
    expires_at: z.number().nullable().optional(),
    credential_extras: z.record(z.string(), z.unknown()).optional(),
    extra: z.record(z.string(), z.unknown()).optional(),
  }),
});
export const templateNameSchema = templateSchema.pick({ name: true });
export type TemplateNameValues = z.infer<typeof templateNameSchema>;
export const maintenanceSchema = z
  .object({
    reauthorize_with_profiles: z.boolean().optional(),
    enabled: z.boolean(),
    interval_minutes: z
      .number()
      .int()
      .min(1, "检查间隔至少为 1 分钟")
      .max(1440, "检查间隔不能超过 1440 分钟"),
    cooldown_minutes: z
      .number()
      .int()
      .min(0, "冷却时间不能小于 0 分钟")
      .max(1440, "冷却时间不能超过 1440 分钟"),
    group_ids: z.array(z.string().min(1)),
    check_after_repair: z.boolean(),
    model: z.string().trim().min(1, "请填写维护模型").max(200),
  })
  .refine((value) => !value.check_after_repair || !!value.model, {
    path: ["model"],
    message: "启用修复后检测时请填写模型",
  });
export type MaintenanceValues = z.infer<typeof maintenanceSchema>;

export const exportSchema = z.object({
  account_ids: z.array(z.string().min(1)).min(1, "请选择至少一个账号"),
});
export type ExportValues = z.infer<typeof exportSchema>;

export const retrySchema = z.object({
  template_id: z.string(),
  model: z.string().trim().min(1, "请填写重试检测模型").max(200, "模型名称不能超过 200 个字符"),
});
export type RetryValues = z.infer<typeof retrySchema>;
