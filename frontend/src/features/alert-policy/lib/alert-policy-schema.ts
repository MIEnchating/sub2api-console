import { z } from "zod";

const routingDegradedTypeSchema = z.enum([
  "health_score",
  "gateway_error_rate",
  "latency",
  "other",
]);

const recoveryNotificationTypeSchema = z.enum([
  "configuration",
  "auth",
  "rate_sync",
  "balance",
  "probe",
  "routing_breaker",
  "routing_degraded",
  "routing_survivor",
  "group_unavailable",
  "group_survivor",
  "apply_failure",
]);

export const alertPolicyFormSchema = z.object({
  enabled: z.boolean(),
  configuration_enabled: z.boolean(),
  auth_enabled: z.boolean(),
  rate_sync_enabled: z.boolean(),
  balance_enabled: z.boolean(),
  probe_enabled: z.boolean(),
  routing_breaker_enabled: z.boolean(),
  routing_degraded_enabled: z.boolean(),
  routing_degraded_types: z.array(routingDegradedTypeSchema),
  routing_survivor_enabled: z.boolean(),
  group_unavailable_enabled: z.boolean(),
  group_survivor_enabled: z.boolean(),
  apply_failure_enabled: z.boolean(),
  balance_thresholds: z
    .array(
      z.object({
        value: z
          .string()
          .trim()
          .min(1, "请输入余额阈值")
          .refine(
            (value) => Number.isFinite(Number(value)) && Number(value) > 0,
            "余额阈值必须是大于 0 的数字",
          ),
      }),
    )
    .min(1, "至少保留一个余额阈值")
    .max(20, "最多设置 20 个余额阈值"),
  probe_failure_streak: z
    .number()
    .int()
    .min(1, "至少连续失败 1 次")
    .max(100, "最多连续失败 100 次"),
  probe_recovery_streak: z
    .number()
    .int()
    .min(1, "至少连续成功 1 次")
    .max(100, "最多连续成功 100 次"),
  probe_groups: z.string(),
  delivery_enabled: z.boolean(),
  notify_recovery: z.boolean(),
  recovery_notification_types: z.array(recoveryNotificationTypeSchema),
  repeat_interval_minutes: z.number().int().min(0, "不能小于 0 分钟").max(10080, "不能超过 7 天"),
  state_change_cooldown_minutes: z
    .number()
    .int()
    .min(0, "不能小于 0 分钟")
    .max(10080, "不能超过 7 天"),
  merge_threshold: z.number().int().min(2, "至少 2 条").max(500, "不能超过 500 条"),
});

export type AlertPolicyFormValues = z.infer<typeof alertPolicyFormSchema>;

export const defaultAlertPolicyForm: AlertPolicyFormValues = {
  enabled: true,
  configuration_enabled: true,
  auth_enabled: true,
  rate_sync_enabled: true,
  balance_enabled: true,
  probe_enabled: true,
  routing_breaker_enabled: true,
  routing_degraded_enabled: true,
  routing_degraded_types: ["health_score", "gateway_error_rate", "latency", "other"],
  routing_survivor_enabled: true,
  group_unavailable_enabled: true,
  group_survivor_enabled: true,
  apply_failure_enabled: true,
  balance_thresholds: [{ value: "20" }, { value: "10" }, { value: "5" }],
  probe_failure_streak: 3,
  probe_recovery_streak: 3,
  probe_groups: "",
  delivery_enabled: true,
  notify_recovery: true,
  recovery_notification_types: ["auth", "balance", "group_unavailable"],
  repeat_interval_minutes: 0,
  state_change_cooldown_minutes: 30,
  merge_threshold: 10,
};
