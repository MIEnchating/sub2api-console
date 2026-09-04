import type { AlertIncident } from "@/api";

const alertTypeLabels: Record<string, string> = {
  "upstream.configuration": "上游配置有问题",
  "upstream.auth": "上游鉴权失败",
  "upstream.rate_sync": "上游倍率同步失败",
  "upstream.balance": "上游余额不足",
  "account.multiplier_increased": "账号倍率上涨",
  "account.multiplier_decreased": "账号倍率下降",
  "account.probe": "账号主动探测失败",
  "account.routing_breaker": "账号触发熔断判定",
  "account.binding_invalid": "账号绑定已失效",
  "account.routing_degraded": "账号进入降级状态",
  "account.routing_survivor": "账号被保底强留",
  "group.routing_unavailable": "分组无可调度账号",
  "group.routing_survivor": "分组仅剩保底账号",
  "routing.apply_failure": "自动执行失败",
};

const alertSubjectLabels: Record<string, string> = {
  "upstream.configuration": "上游配置",
  "upstream.auth": "上游鉴权",
  "upstream.rate_sync": "上游倍率同步",
  "upstream.balance": "上游余额",
  "account.multiplier_increased": "账号倍率",
  "account.multiplier_decreased": "账号倍率",
  "account.probe": "账号主动探测",
  "account.routing_breaker": "账号调度",
  "account.binding_invalid": "账号绑定",
  "account.routing_degraded": "账号降级",
  "account.routing_survivor": "账号保底强留",
  "group.routing_unavailable": "分组可用性",
  "group.routing_survivor": "分组保底",
  "routing.apply_failure": "自动执行",
};

const causeLabels: Record<string, string> = {
  CONFIG: "上游配置有问题",
  CONFIG_METADATA_INVALID: "上游返回信息无法识别",
  CONFIG_AUTH_STATUS_MISSING: "上游没有返回鉴权状态",
  CONFIG_AUTH_STATUS_UNKNOWN: "上游鉴权状态无法识别",
  CONFIG_BALANCE_CLOSED_INVALID: "上游余额关闭状态无效",
  CONFIG_BALANCE_INVALID: "上游余额不是有效数字",
  AUTH: "上游鉴权失败",
  RATE_SYNC: "上游倍率同步失败",
  BALANCE_HARD_CLOSED: "上游因余额不足停止服务",
  PROBE: "连续主动探测失败",
  ROUTING_BREAKER: "调度策略触发熔断判定",
  BINDING_INVALID: "绑定的上游或分组已不存在",
  ROUTING_DEGRADED: "调度策略判定为降级",
  ROUTING_DEGRADED_HEALTH_SCORE: "健康分低于降级线",
  ROUTING_DEGRADED_GATEWAY_ERROR_RATE: "网关错误率达到降级阈值",
  ROUTING_DEGRADED_LATENCY: "响应延迟达到降级阈值",
  ROUTING_DEGRADED_OTHER: "其他调度降级原因",
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

function compactRateSyncReason(value: string): string {
  let reason = value.trim();
  for (const prefix of ["上游倍率同步失败：", "倍率同步失败："]) {
    while (reason.startsWith(prefix)) reason = reason.slice(prefix.length).trim();
  }
  if (!reason) return "未能读取上游倍率信息";

  const statuses = new Set(
    Array.from(reason.matchAll(/HTTP\s+([1-5][0-9]{2})/g), (match) => match[1]),
  );
  const scopes: string[] = [];
  if (reason.includes("分组目录读取失败")) scopes.push("分组目录");
  if (reason.includes("余额读取失败")) scopes.push("余额");
  if (statuses.size === 0 && scopes.length > 0 && reason.includes("上游网络请求失败")) {
    return `上游网络请求失败，${scopes.join("和")}读取失败`;
  }
  if (statuses.size === 1 && scopes.length > 0) {
    return `上游返回 HTTP ${Array.from(statuses)[0]}，${scopes.join("和")}读取失败`;
  }
  if (statuses.size > 0 && scopes.length > 0) {
    const details = reason
      .split("；")
      .map((part) => {
        let scope = "";
        if (part.includes("分组目录读取失败")) scope = "分组目录";
        else if (part.includes("余额读取失败")) scope = "余额";
        if (!scope) return "";
        const status = part.match(/HTTP\s+([1-5][0-9]{2})/)?.[1];
        return status ? `${scope}（HTTP ${status}）` : scope;
      })
      .filter(Boolean);
    if (details.length > 0) return `${details.join("、")}读取失败`;
  }
  if (statuses.size === 1) {
    return `上游返回 HTTP ${Array.from(statuses)[0]}，倍率同步未完成`;
  }

  const characters = Array.from(reason);
  return characters.length > 120 ? `${characters.slice(0, 119).join("")}…` : reason;
}

export function alertTypeLabel(eventType: string, status?: string): string {
  if (status === "recovered") {
    const recoveredLabels: Record<string, string> = {
      "upstream.configuration": "上游配置已恢复",
      "upstream.auth": "上游鉴权已恢复",
      "upstream.rate_sync": "上游倍率同步已恢复",
      "upstream.balance": "上游余额已恢复",
      "account.probe": "账号主动探测已恢复",
      "account.routing_breaker": "账号已恢复调度",
      "account.binding_invalid": "账号绑定已恢复",
      "account.routing_degraded": "账号已恢复正常调度",
      "account.routing_survivor": "账号已退出临时保留",
      "group.routing_unavailable": "分组已恢复可用",
      "group.routing_survivor": "分组已恢复正常",
      "routing.apply_failure": "自动处理已恢复",
    };
    if (recoveredLabels[eventType]) return recoveredLabels[eventType];
  }
  return alertTypeLabels[eventType] ?? unknownLabel("其他告警", eventType);
}

export function alertSubjectLabel(eventType: string): string {
  return alertSubjectLabels[eventType] ?? unknownLabel("其他告警", eventType);
}

export function alertCauseLabel(causeCode: string, status?: string): string {
  for (const change of [
    { prefix: "MULTIPLIER_INCREASED:", verb: "上涨" },
    { prefix: "MULTIPLIER_DECREASED:", verb: "下降" },
  ]) {
    if (!causeCode.startsWith(change.prefix)) continue;
    const values = causeCode
      .slice(change.prefix.length)
      .split(" -> ", 2)
      .map((value) => value.trim());
    if (values.length !== 2 || !values[0] || !values[1]) return "倍率发生变化";
    return `倍率从 ${values[0]} ${change.verb}至 ${values[1]}`;
  }
  if (causeCode.startsWith("BALANCE:")) {
    const threshold = causeCode.slice("BALANCE:".length).trim();
    if (status === "recovered") {
      return threshold ? `余额已高于告警阈值 ${threshold}` : "余额已恢复至告警阈值以上";
    }
    return threshold ? `余额已达到或低于告警阈值 ${threshold}` : "余额已达到或低于告警阈值";
  }
  if (status === "recovered" && causeCode === "BALANCE_HARD_CLOSED") {
    return "余额不足导致的停服已解除";
  }
  if (status === "recovered") {
    const code = causeCode.split(":", 1)[0];
    const recoveredCauses: Record<string, string> = {
      CONFIG: "上游配置已恢复正常",
      AUTH: "上游鉴权已恢复",
      RATE_SYNC: "倍率同步已恢复",
      PROBE: "账号主动探测已恢复",
      ROUTING_BREAKER: "账号已恢复调度",
      BINDING_INVALID: "账号绑定已恢复",
      ROUTING_DEGRADED: "账号已恢复正常调度",
      ROUTING_DEGRADED_HEALTH_SCORE: "账号已恢复正常调度",
      ROUTING_DEGRADED_GATEWAY_ERROR_RATE: "账号已恢复正常调度",
      ROUTING_DEGRADED_LATENCY: "账号已恢复正常调度",
      ROUTING_DEGRADED_OTHER: "账号已恢复正常调度",
      ROUTING_SURVIVOR: "账号已退出临时保留",
      GROUP_UNAVAILABLE: "分组已恢复可用",
      GROUP_SURVIVOR_ONLY: "分组已恢复正常",
      APPLY_FAILED: "自动处理已恢复",
    };
    return recoveredCauses[code] ?? "相关问题已恢复";
  }
  if (causeCode.startsWith("RATE_SYNC:")) {
    const reason = causeCode.slice("RATE_SYNC:".length).trim();
    return compactRateSyncReason(reason);
  }
  for (const code of [
    "AUTH",
    "PROBE",
    "CONFIG_AUTH_STATUS_UNKNOWN",
    "CONFIG_BALANCE_INVALID",
    "ROUTING_BREAKER",
    "BINDING_INVALID",
    "ROUTING_DEGRADED",
    "ROUTING_DEGRADED_HEALTH_SCORE",
    "ROUTING_DEGRADED_GATEWAY_ERROR_RATE",
    "ROUTING_DEGRADED_LATENCY",
    "ROUTING_DEGRADED_OTHER",
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
