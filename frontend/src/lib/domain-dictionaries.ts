export type DictionaryOption<T extends string = string> = Readonly<{
  value: T;
  label: string;
}>;

export type DictionaryEntryLike = Readonly<{
  value: string;
  name: string;
  enabled: boolean;
}>;

/** Build options from the managed dictionary while retaining unseen runtime values. */
export function orderedDictionaryOptions(
  entries: readonly DictionaryEntryLike[] | undefined,
  discovered: readonly DictionaryOption[],
): DictionaryOption[] {
  if (!entries) return [...discovered];
  const known = new Set<string>();
  const options: DictionaryOption[] = [];
  for (const entry of entries) {
    const value = entry.value.trim().toLocaleLowerCase();
    if (!value || known.has(value)) continue;
    known.add(value);
    if (entry.enabled) options.push({ value, label: entry.name.trim() || value });
  }
  for (const option of discovered) {
    const value = option.value.trim().toLocaleLowerCase();
    if (!value || known.has(value)) continue;
    known.add(value);
    options.push({ value, label: option.label || value });
  }
  return options;
}

export type StatusTone = "success" | "warning" | "danger" | "info" | "neutral";

export const schedulingStrategyDictionary: Readonly<Record<string, string>> = {
  balanced: "均衡",
  price: "价格优先",
  price_first: "价格优先",
  cost_first: "价格优先",
  speed: "速度优先",
  latency_first: "速度优先",
  speed_first: "速度优先",
  reliability: "稳定优先",
  reliability_first: "稳定优先",
  stable: "稳定优先",
  stability: "稳定优先",
  stability_first: "稳定优先",
  未参与: "未参与",
  配置错误: "配置错误",
} as const satisfies Readonly<Record<string, string>>;

export const taskStatusDictionary: Readonly<Record<string, string>> = {
  queued: "排队中",
  running: "进行中",
  waiting_input: "等待输入",
  succeeded: "已成功",
  partial: "部分完成",
  failed: "已失败",
  cancelled: "已取消",
} as const;

export const concurrencyLimitedLabel = "等待并发额度";

export const runtimeStatusDictionary: Readonly<Record<string, string>> = {
  manual_priority: "人工优先位",
  ok: "正常",
  partial: "部分完成",
  warning: "警告",
  记录: "已记录",
  queued: "排队中",
  running: "运行中",
  waiting_input: "等待输入",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
  error: "错误",
  healthy: "健康",
  degraded: "降级",
  fused: "熔断",
  cost_blocked: "成本墙拦截",
  concurrency_limited: concurrencyLimitedLabel,
  survivor: "保底",
  paused: "已暂停",
  excluded: "已排除",
  unknown: "待观察",
  active: "生效",
  inactive: "未生效",
  enabled: "已启用",
  disabled: "已停用",
  available: "可用",
  unavailable: "不可用",
  authenticated: "已鉴权",
  unauthenticated: "未鉴权",
  synced: "已同步",
  unsynced: "未同步",
  insufficient: "余额不足",
  exhausted: "额度耗尽",
  not_found: "未找到",
  not_read: "未读取",
  pending: "待处理",
  firing: "告警中",
  recovered: "已恢复",
  suppressed: "规则已停用",
  closed: "已关闭",
  sent: "已发送",
  delivered: "已发送",
  failed_delivery: "通知发送失败",
  credential_invalid: "鉴权失效",
  auth_recovery_failed: "鉴权恢复失败",
  refresh_token_invalid: "刷新令牌失效",
  image_captcha_required: "需要人工完成图片验证码",
  image_captcha_ocr: "等待输入图片验证码",
  browser_challenge_required: "需要浏览器验证",
  probe_failed: "探测失败",
  gateway_error: "网关错误",
  rate_limited_or_exhausted: "限流或额度不足",
  unknown_upstream_error: "上游错误",
  empty_response: "疑似空回复",
  apply: "自动执行",
  shadow: "仅计算",
  calculation: "计算",
  write: "写入",
  readback: "读回",
  remote_write: "远程写入",
  remote_readback: "远程读回",
  evaluate: "评估",
  inspect: "检查",
  true: "开启",
  false: "关闭",
};

export const fallbackModeDictionary = {
  current_cost_wall: "回退当前成本墙",
  fail_closed: "严格关闭",
  fail_open: "允许继续",
} as const;

export const autoApplyFieldDictionary = {
  schedulable: "调度状态",
  priority: "优先级",
  load_factor: "负载因子",
  concurrency: "并发上限",
} as const;

export const trafficTimeRangeOptions = [
  { value: "1h", label: "最近 1 小时" },
  { value: "6h", label: "最近 6 小时" },
  { value: "24h", label: "最近 24 小时" },
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
] as const;

export const trafficRankingSortOptions = [
  { value: "traffic", label: "按流量" },
  { value: "stability", label: "按稳定性" },
  { value: "success_rate", label: "按成功率" },
  { value: "latency", label: "按 P95 延迟" },
] as const;

export const accountStateDictionary: Readonly<Record<string, string>> = {
  manual_priority: "人工优先位",
  healthy: "健康",
  active: "健康",
  available: "可用",
  degraded: "降级",
  survivor: "保底",
  fused: "熔断",
  cost_blocked: "成本墙拦截",
  concurrency_limited: concurrencyLimitedLabel,
  paused: "暂停",
  disabled: "不可用",
  excluded: "已排除",
  unknown: "待探测",
};

export const kumaRequestAuthDictionary: Readonly<Record<string, string>> = {
  none: "无鉴权",
  basic: "Basic",
  bearer: "Bearer",
} as const;

export const kumaMonitorTypeDictionary: Readonly<Record<string, string>> = {
  http: "HTTP(S)",
  keyword: "HTTP(S) 关键字",
  port: "TCP 端口",
  ping: "Ping",
  dns: "DNS",
  push: "Push 推送",
  group: "分组",
};

export const kumaNotificationTypeDictionary: Readonly<Record<string, string>> = {
  webhook: "Webhook",
  telegram: "Telegram",
  smtp: "邮件（SMTP）",
  ntfy: "ntfy",
  discord: "Discord",
};

export const kumaMaintenanceStrategyDictionary: Readonly<Record<string, string>> = {
  manual: "手动维护",
  single: "单次维护",
  cron: "Cron 定时",
  "recurring-interval": "按天间隔",
  "recurring-weekday": "每周重复",
  "recurring-day-of-month": "每月重复",
};

export const alertObjectKindDictionary: Readonly<Record<string, string>> = {
  account: "账号",
  host: "上游",
  group: "分组",
};
export const alertStatusDictionary: Readonly<Record<string, string>> = {
  firing: "告警中",
  recovered: "已恢复",
  suppressed: "规则已停用",
  closed: "已关闭",
} as const;

export const groupStatusDictionary: Readonly<Record<string, { label: string; tone: StatusTone }>> =
  {
    healthy: { label: "健康", tone: "success" },
    rate_limited: { label: "限流中", tone: "warning" },
    partial_degraded: { label: "部分异常", tone: "warning" },
    survivor_only: { label: "仅剩保底", tone: "danger" },
    all_fused: { label: "全部熔断", tone: "danger" },
    all_unavailable: { label: "全部不可调度", tone: "danger" },
    excluded: { label: "已排除", tone: "neutral" },
    skipped: { label: "未参与", tone: "neutral" },
    empty: { label: "无账号", tone: "neutral" },
  };

export function dictionaryLabel(
  dictionary: Readonly<Record<string, string>>,
  value: string | null | undefined,
  fallback = "配置错误",
): string {
  const normalized = value?.trim();
  if (!normalized || !Object.hasOwn(dictionary, normalized)) return fallback;
  return dictionary[normalized] || fallback;
}

export const upstreamTypeOptions = [
  { value: "sub2api", label: "Sub2API" },
  { value: "newapi", label: "New API" },
  { value: "oneapi", label: "OneAPI" },
  { value: "custom", label: "自定义上游" },
  { value: "apikey", label: "API Key" },
] as const satisfies readonly DictionaryOption[];

export const configurableUpstreamTypeOptions = upstreamTypeOptions.filter(
  (option) => option.value !== "apikey",
);

export const upstreamAuthStatuses = {
  authenticated: "已鉴权",
  recovered: "已恢复",
  pendingVerification: "待验证",
  unconfirmed: "未确认",
  recoveryTemporarilyFailed: "恢复暂时失败",
  invalid: "鉴权失效",
  configurationError: "配置错误",
} as const;

export const upstreamAuthStatusOptions = [
  {
    value: upstreamAuthStatuses.authenticated,
    label: upstreamAuthStatuses.authenticated,
    tone: "success",
  },
  {
    value: upstreamAuthStatuses.recovered,
    label: upstreamAuthStatuses.recovered,
    tone: "success",
  },
  {
    value: upstreamAuthStatuses.pendingVerification,
    label: upstreamAuthStatuses.pendingVerification,
    tone: "warning",
  },
  {
    value: upstreamAuthStatuses.unconfirmed,
    label: upstreamAuthStatuses.unconfirmed,
    tone: "neutral",
  },
  {
    value: upstreamAuthStatuses.recoveryTemporarilyFailed,
    label: upstreamAuthStatuses.recoveryTemporarilyFailed,
    tone: "warning",
  },
  {
    value: upstreamAuthStatuses.invalid,
    label: upstreamAuthStatuses.invalid,
    tone: "danger",
  },
  {
    value: upstreamAuthStatuses.configurationError,
    label: upstreamAuthStatuses.configurationError,
    tone: "danger",
  },
] as const satisfies readonly (DictionaryOption & { tone: StatusTone })[];

export const accountTypeOptions = [
  { value: "apikey", label: "API Key" },
  { value: "oauth", label: "OAuth" },
  { value: "sub2api", label: "Sub2API" },
  { value: "newapi", label: "New API" },
  { value: "oneapi", label: "OneAPI" },
] as const satisfies readonly DictionaryOption[];

const upstreamTypeByValue = new Map<string, DictionaryOption>(
  upstreamTypeOptions.map((option) => [option.value, option]),
);
const upstreamAuthStatusByValue = new Map<string, DictionaryOption & { tone: StatusTone }>(
  upstreamAuthStatusOptions.map((option) => [option.value, option]),
);
const accountTypeByValue = new Map<string, DictionaryOption>(
  accountTypeOptions.map((option) => [option.value, option]),
);
const accountTypeAliases: Readonly<Record<string, string>> = {
  api_key: "apikey",
  "api-key": "apikey",
};
const readyUpstreamAuthStatuses = new Set([
  upstreamAuthStatuses.authenticated,
  upstreamAuthStatuses.recovered,
  "已发现鉴权记录",
  "已认证",
  "authenticated",
  "authorized",
  "healthy",
  "valid",
  "ok",
  "succeeded",
]);

export function knownUpstreamTypeLabel(value: string | null | undefined): string | null {
  const normalized = value?.trim().toLowerCase();
  if (!normalized) return null;
  return upstreamTypeByValue.get(normalized)?.label ?? null;
}

export function upstreamTypeLabel(value: string | null | undefined): string {
  const raw = value?.trim();
  if (!raw) return "未配置";
  return knownUpstreamTypeLabel(raw) ?? `未知类型（${raw}）`;
}

export function upstreamAuthStatusMeta(value: string | null | undefined): {
  label: string;
  tone: StatusTone;
} {
  const normalized = value?.trim();
  if (!normalized) return { label: upstreamAuthStatuses.unconfirmed, tone: "neutral" };
  const known = upstreamAuthStatusByValue.get(normalized);
  if (known) return { label: known.label, tone: known.tone };
  return { label: `未知状态（${normalized}）`, tone: "neutral" };
}

export function upstreamAuthStatusIsReady(value: string | null | undefined): boolean {
  const normalized = value?.trim().toLowerCase();
  return normalized ? readyUpstreamAuthStatuses.has(normalized) : false;
}

export function accountTypeValue(value: string | null | undefined): string | null {
  const normalized = value?.trim().toLowerCase();
  if (!normalized) return null;
  return Object.hasOwn(accountTypeAliases, normalized)
    ? accountTypeAliases[normalized]
    : normalized;
}

export function accountTypeLabel(value: string | null | undefined): string | null {
  const raw = value?.trim();
  const normalized = accountTypeValue(raw);
  if (!normalized) return null;
  return accountTypeByValue.get(normalized)?.label ?? `未知类型（${raw}）`;
}
