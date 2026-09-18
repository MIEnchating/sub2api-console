import { z } from "zod";
import type { TemplateConfig, WorkbenchTemplate } from "../types";

const decimal = z
  .string()
  .trim()
  .regex(/^\d+(\.\d+)?$/, "请输入非负数，使用小数点分隔");
const positiveId = z.string().regex(/^[1-9]\d{0,17}$/, "请输入有效的正整数 ID");
const mapping = z.string().superRefine((text, ctx) => {
  try {
    const value: unknown = JSON.parse(text);
    if (
      !value ||
      typeof value !== "object" ||
      Array.isArray(value) ||
      Object.keys(value).length > 1000 ||
      Object.values(value).some((item) => typeof item !== "string")
    )
      ctx.addIssue({ code: "custom", message: "模型映射必须是模型名到模型名的 JSON 对象" });
  } catch {
    ctx.addIssue({ code: "custom", message: "请输入有效的 JSON 对象" });
  }
});
export const manualTemplateSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "请输入模板名称")
    .max(60, "模板名称最多 60 字")
    .regex(/^[^\r\n\0]*$/, "名称不能包含换行"),
  concurrency: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
  priority: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
  rate_multiplier: decimal,
  load_factor: z.union([
    z.literal(""),
    decimal.refine((value) => /[1-9]/.test(value), "负载因子必须大于 0"),
  ]),
  proxy_id: z.union([z.literal(""), positiveId]),
  group_ids: z
    .string()
    .trim()
    .refine(
      (value) => !value || value.split(/[,，\s]+/).every((id) => positiveId.safeParse(id).success),
      "请输入分组 ID，多个 ID 用逗号分隔",
    ),
  plan: z.string(),
  fingerprint: z.enum(["off", "device", "session", "full"]),
  model_mapping: mapping,
  auto_pause_on_expired: z.boolean(),
  notes: z.string().max(4000, "备注最多 4000 字"),
});
export type ManualTemplateValues = z.infer<typeof manualTemplateSchema>;
export function manualTemplateDefaults(template?: WorkbenchTemplate): ManualTemplateValues {
  const c = template?.config;
  return {
    name: template?.name ?? "",
    concurrency: c?.concurrency ?? 1,
    priority: c?.priority ?? 50,
    rate_multiplier: c?.rate_multiplier ?? "1",
    load_factor: c?.load_factor ?? "",
    proxy_id: c?.proxy_id ?? "",
    group_ids: c?.group_ids.join(", ") ?? "",
    plan: typeof c?.credential_extras.plan_type === "string" ? c.credential_extras.plan_type : "",
    fingerprint:
      manualTemplateSchema.shape.fingerprint.safeParse(c?.extra.codex_fingerprint_mode).data ??
      "off",
    model_mapping: JSON.stringify(c?.credential_extras.model_mapping ?? {}, null, 2),
    auto_pause_on_expired: c?.auto_pause_on_expired ?? true,
    notes: c?.notes ?? "",
  };
}
export function manualTemplateConfig(
  values: ManualTemplateValues,
  original?: TemplateConfig,
): TemplateConfig {
  const extras = { ...original?.credential_extras };
  if (values.plan) extras.plan_type = values.plan;
  else delete extras.plan_type;
  extras.model_mapping = JSON.parse(values.model_mapping) as Record<string, string>;
  return {
    ...original,
    concurrency: values.concurrency,
    priority: values.priority,
    rate_multiplier: values.rate_multiplier,
    load_factor: values.load_factor || null,
    proxy_id: values.proxy_id || null,
    group_ids: values.group_ids ? [...new Set(values.group_ids.split(/[,，\s]+/))] : [],
    auto_pause_on_expired: values.auto_pause_on_expired,
    expires_at: original?.expires_at ?? null,
    notes: values.notes,
    credential_extras: extras,
    extra: { ...original?.extra, codex_fingerprint_mode: values.fingerprint },
  };
}
