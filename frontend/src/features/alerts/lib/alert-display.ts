import type { AlertIncident } from "@/api";

const alertTypeLabels: Record<string, string> = {
  "upstream.configuration": "上游配置异常",
  "upstream.auth": "上游鉴权失效",
  "upstream.rate_sync": "上游倍率同步失败",
  "upstream.balance": "上游余额不足",
  "account.probe": "账号主动探测失败",
  "account.routing_breaker": "账号触发熔断判定",
  "account.routing_degraded": "账号进入降级状态",
  "account.routing_survivor": "账号被保底强留",
  "group.routing_unavailable": "分组无可调度账号",
  "group.routing_survivor": "分组仅剩保底账号",
  "routing.apply_failure": "自动执行失败",
};

const causeLabels: Record<string, string> = {
  CONFIG: "上游配置不完整或无效",
  CONFIG_METADATA_INVALID: "上游元数据无法解析",
  CONFIG_AUTH_STATUS_MISSING: "上游鉴权状态缺失",
  CONFIG_AUTH_STATUS_UNKNOWN: "上游鉴权状态无法识别",
  CONFIG_BALANCE_CLOSED_INVALID: "余额关闭状态格式无效",
  CONFIG_BALANCE_INVALID: "上游余额不是有效数字",
  AUTH: "上游鉴权失效",
  RATE_SYNC: "上游倍率同步失败",
  BALANCE_HARD_CLOSED: "上游因余额不足已关闭服务",
  PROBE: "连续主动探测失败",
  ROUTING_BREAKER: "调度策略触发熔断判定",
  ROUTING_DEGRADED: "调度策略判定为降级",
  ROUTING_SURVIVOR: "为避免分组断供而保底强留",
  GROUP_UNAVAILABLE: "调度判定后没有可调度账号",
  GROUP_SURVIVOR_ONLY: "调度判定后仅剩保底账号",
  APPLY_FAILED: "自动执行未成功",
};

const objectKindLabels: Record<string, string> = {
  account: "账号",
  host: "上游",
  group: "分组",
};

const alertStatusLabels: Record<string, string> = {
  firing: "告警中",
  recovered: "已恢复",
  suppressed: "规则已停用",
  closed: "已关闭",
};

const deliveryStatusLabels: Record<string, string> = {
  sent: "通知已发送",
  delivered: "通知已送达",
  failed: "通知发送失败",
  pending: "通知待发送",
  已发送: "通知已发送",
  发送失败: "通知发送失败",
  未配置渠道: "未配置通知渠道",
  渠道不支持: "通知渠道不支持",
  通知配置无效: "通知配置无效",
  投递已关闭: "通知发送已关闭",
  通知发送已关闭: "通知发送已关闭",
  恢复通知已关闭: "恢复通知已关闭",
  规则已停用: "告警规则已停用",
  告警总开关已关闭: "告警总开关已关闭",
  停用期间异常已消失: "停用期间异常已消失",
  告警档位已变化: "告警档位已变化",
  余额告警阈值档位已变化: "余额告警阈值档位已变化",
  同一对象的其他异常仍存在: "同一对象的其他异常仍存在",
  主动探测证据不足或已过期: "主动探测证据不足或已过期",
  "运行模式已切换，旧调度判定已失效": "运行模式已切换，旧调度判定已失效",
};

function unknownLabel(prefix: string, value: string | null | undefined) {
  const normalized = value?.trim();
  return normalized ? `${prefix}（${normalized}）` : prefix;
}

export function alertTypeLabel(eventType: string, status?: string): string {
  if (status === "recovered" && eventType === "upstream.balance") return "上游余额恢复";
  return alertTypeLabels[eventType] ?? unknownLabel("其他告警", eventType);
}

export function alertCauseLabel(causeCode: string, status?: string): string {
  if (causeCode.startsWith("BALANCE:")) {
    const threshold = causeCode.slice("BALANCE:".length).trim();
    if (status === "recovered") {
      return threshold ? `余额已高于告警阈值 ${threshold}` : "余额已恢复至告警阈值以上";
    }
    return threshold ? `余额已达到或低于告警阈值 ${threshold}` : "余额已达到或低于告警阈值";
  }
  if (status === "recovered" && causeCode === "BALANCE_HARD_CLOSED") {
    return "余额不足关闭状态已解除";
  }
  if (causeCode.startsWith("RATE_SYNC:")) {
    const reason = causeCode.slice("RATE_SYNC:".length).trim();
    return reason ? `上游倍率同步失败：${reason}` : "上游倍率同步失败";
  }
  for (const code of [
    "AUTH",
    "PROBE",
    "CONFIG_AUTH_STATUS_UNKNOWN",
    "CONFIG_BALANCE_INVALID",
    "ROUTING_BREAKER",
    "ROUTING_DEGRADED",
    "ROUTING_SURVIVOR",
    "APPLY_FAILED",
  ]) {
    const prefix = `${code}:`;
    if (!causeCode.startsWith(prefix)) continue;
    const reason = causeCode.slice(prefix.length).trim();
    const label = causeLabels[code] ?? code;
    return reason ? `${label}：${reason}` : label;
  }
  return causeLabels[causeCode] ?? unknownLabel("未分类原因", causeCode);
}

function accountGroupPrefix(eventType: string, objectId: string): string {
  const prefixes: Record<string, string> = {
    "account.probe": "console:probe:",
    "account.routing_breaker": "console:routing:breaker:",
    "account.routing_degraded": "console:routing:degraded:",
    "account.routing_survivor": "console:routing:survivor:",
  };
  const prefix = prefixes[eventType];
  return prefix ? `${prefix}${objectId}:` : "";
}

export function alertObjectLabel(
  alert: Pick<
    AlertIncident,
    "incident_key" | "event_type" | "object_kind" | "object_id" | "object_name"
  >,
) {
  const objectId = String(alert.object_id ?? "").trim();
  if (alert.object_kind === "account") {
    const account = alert.object_name
      ? `${alert.object_name}（账号 #${objectId}）`
      : `账号 #${objectId}`;
    const prefix = accountGroupPrefix(alert.event_type, objectId);
    const group =
      prefix !== "" && alert.incident_key.startsWith(prefix)
        ? alert.incident_key.slice(prefix.length).trim()
        : "";
    return group ? `${account} · 分组 ${group}` : account;
  }
  if (objectId) return objectId;
  return objectKindLabels[alert.object_kind] ?? unknownLabel("告警对象", alert.object_kind);
}

export function alertStatusLabel(status: string) {
  return alertStatusLabels[status] ?? unknownLabel("状态未知", status);
}

export function alertDeliveryLabel(status: string | null | undefined, attempts = 0) {
  if (!status) return "通知状态未知";
  const label = deliveryStatusLabels[status] ?? unknownLabel("通知状态未知", status);
  if (attempts > 0 && ["sent", "delivered", "已发送"].includes(status)) {
    return `${label} ${attempts} 次`;
  }
  return label;
}
