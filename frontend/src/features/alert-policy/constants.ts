import type { AlertPolicyFormValues } from "./lib/alert-policy-schema";

export const alertRuleFields: Array<{
  name:
    | "configuration_enabled"
    | "auth_enabled"
    | "rate_sync_enabled"
    | "balance_enabled"
    | "probe_enabled"
    | "routing_breaker_enabled"
    | "routing_survivor_enabled"
    | "group_unavailable_enabled"
    | "group_survivor_enabled"
    | "apply_failure_enabled";
  label: string;
  description: string;
}> = [
  {
    name: "configuration_enabled",
    label: "配置异常",
    description: "上游状态、余额或元数据无法解析时产生告警",
  },
  { name: "auth_enabled", label: "鉴权失效", description: "Token 失效、过期、未鉴权或恢复失败" },
  { name: "rate_sync_enabled", label: "倍率同步失败", description: "最近一次上游倍率同步任务失败" },
  { name: "balance_enabled", label: "余额不足", description: "余额达到阈值或上游已触发余额硬关闭" },
  {
    name: "probe_enabled",
    label: "主动探测失败",
    description: "账号在分组中的连续主动探测结果未通过",
  },
  {
    name: "routing_breaker_enabled",
    label: "账号熔断判定",
    description: "调度策略判定账号需要停止接收流量",
  },
  {
    name: "routing_survivor_enabled",
    label: "保底强留",
    description: "账号本应熔断，但为避免分组断供而继续保留",
  },
  {
    name: "group_unavailable_enabled",
    label: "分组无可调度账号",
    description: "本轮调度判定后，分组内没有账号可以接收流量",
  },
  {
    name: "group_survivor_enabled",
    label: "分组仅剩保底账号",
    description: "本轮调度判定后，分组仅靠保底账号维持服务",
  },
  {
    name: "apply_failure_enabled",
    label: "自动执行失败",
    description: "调度目标写入远端失败，或写入后无法确认实际状态",
  },
];

export const routingDegradedFields: Array<{
  value: AlertPolicyFormValues["routing_degraded_types"][number];
  label: string;
  description: string;
}> = [
  { value: "health_score", label: "健康分过低", description: "健康分低于调度策略的降级线" },
  {
    value: "gateway_error_rate",
    label: "网关错误率过高",
    description: "近期网关错误达到仅降级阈值",
  },
  { value: "latency", label: "响应延迟超标", description: "慢响应次数达到仅降级阈值" },
  { value: "other", label: "其他降级原因", description: "无法归入以上类型的降级判定" },
];

export const recoveryNotificationFields: Array<{
  value: AlertPolicyFormValues["recovery_notification_types"][number];
  label: string;
  description: string;
}> = [
  { value: "configuration", label: "配置异常恢复", description: "配置或数据格式重新可用" },
  { value: "auth", label: "鉴权恢复", description: "上游鉴权重新通过" },
  { value: "rate_sync", label: "倍率同步恢复", description: "倍率同步任务重新成功" },
  { value: "balance", label: "余额恢复", description: "余额离开告警区间或解除硬关闭" },
  { value: "probe", label: "主动探测恢复", description: "连续探测重新通过，频繁波动时建议关闭" },
  { value: "routing_breaker", label: "账号熔断恢复", description: "账号重新满足回池条件" },
  {
    value: "routing_degraded",
    label: "账号降级恢复",
    description: "账号退出降级状态，频繁调权时建议关闭",
  },
  { value: "routing_survivor", label: "保底强留恢复", description: "账号不再需要保底强留" },
  { value: "group_unavailable", label: "分组可用性恢复", description: "分组重新出现可调度账号" },
  { value: "group_survivor", label: "分组保底恢复", description: "分组不再仅依赖保底账号" },
  { value: "apply_failure", label: "自动执行恢复", description: "后续远端写入和复核重新成功" },
];

export const notificationTargetTypeLabels: Record<string, string> = {
  c2c: "私聊",
  group: "群聊",
  channel: "频道",
};
