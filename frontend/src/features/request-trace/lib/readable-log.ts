import type { UsageRecord } from "@/api";

function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === "") return "";
  return String(value);
}

function messageValues(message: string): { summary: string; values: Record<string, string> } {
  const values: Record<string, string> = {};
  const expression = /(?:^|\s)([a-zA-Z][\w.-]*)=([^\s]+)/g;
  let firstIndex = -1;
  for (const match of message.matchAll(expression)) {
    if (firstIndex < 0) firstIndex = match.index ?? -1;
    values[match[1]] = match[2];
  }
  return {
    summary: (firstIndex >= 0 ? message.slice(0, firstIndex) : message).trim(),
    values,
  };
}

function payloadValue(record: UsageRecord, keys: string[]): string {
  const extra =
    typeof record.payload.extra === "object" && record.payload.extra !== null
      ? (record.payload.extra as Record<string, unknown>)
      : {};
  for (const key of keys) {
    const direct = displayValue(record.payload[key]);
    if (direct) return direct;
    const nested = displayValue(extra[key]);
    if (nested) return nested;
  }
  return "";
}

function firstValue(values: Record<string, string>, keys: string[]): string {
  for (const key of keys) {
    if (values[key]) return values[key];
  }
  return "";
}

function readableTitle(summary: string): string {
  const normalized = summary.toLowerCase();
  if (normalized === "http request completed") return "HTTP 请求完成";
  if (normalized === "http request started") return "HTTP 请求开始";
  if (normalized === "http request failed") return "HTTP 请求失败";
  if (normalized.startsWith("account test error:")) return "账号测试失败";
  return summary || "系统日志";
}

export function readableDuration(value: string): string {
  const milliseconds = Number(value);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return value || "未记录";
  if (milliseconds < 1000) return `${milliseconds} ms`;
  const seconds = (milliseconds / 1000).toFixed(3).replace(/\.?0+$/, "");
  return `${seconds} 秒 (${milliseconds} ms)`;
}

const tokenFormatter = new Intl.NumberFormat("zh-CN");

function readableTokens(value: string | null | undefined): string {
  if (value === null || value === undefined || value === "") return "未提供";
  if (/^\d+$/.test(value.trim())) return tokenFormatter.format(BigInt(value.trim()));
  const number = Number(value);
  if (!Number.isFinite(number)) return value;
  return tokenFormatter.format(number);
}

export type ReadableUsage = {
  input: string;
  output: string;
  cacheRead: string;
  cacheWrite: string;
};

export function readableUsage(
  record: Pick<
    UsageRecord,
    | "usage_available"
    | "input_tokens"
    | "output_tokens"
    | "cache_read_tokens"
    | "cache_write_tokens"
  >,
): ReadableUsage | null {
  if (!record.usage_available) return null;
  return {
    input: readableTokens(record.input_tokens),
    output: readableTokens(record.output_tokens),
    cacheRead: readableTokens(record.cache_read_tokens),
    cacheWrite: readableTokens(record.cache_write_tokens),
  };
}

export type ReadableSystemLog = {
  title: string;
  status: string;
  duration: string;
  requestId: string;
  clientRequestId: string;
  accountId: string;
  accountName: string;
  account: string;
  apiKeyId: string;
  platform: string;
  model: string;
  method: string;
  path: string;
  ip: string;
  protocol: string;
  host: string;
  level: string;
};

export function readableSystemLog(record: UsageRecord): ReadableSystemLog {
  const message = messageValues(record.summary ?? "");
  const fromMessage = (keys: string[]) => firstValue(message.values, keys);
  const fromPayload = (keys: string[]) => payloadValue(record, keys);
  const accountId =
    displayValue(record.account_id) ||
    fromPayload(["account_id"]) ||
    fromMessage(["acc", "account_id"]);
  const accountName = displayValue(record.account_name);
  return {
    title: readableTitle(message.summary),
    status: fromPayload(["status", "status_code"]) || fromMessage(["status", "status_code"]),
    duration:
      displayValue(record.duration_ms) ||
      fromPayload(["latency_ms", "duration_ms"]) ||
      fromMessage(["latency_ms", "duration_ms"]),
    requestId:
      displayValue(record.request_id) || fromPayload(["request_id"]) || fromMessage(["req"]),
    clientRequestId:
      fromPayload(["client_request_id"]) || fromMessage(["client_req", "client_request_id"]),
    accountId,
    accountName,
    account: readableAccount(accountName, accountId),
    apiKeyId: fromPayload(["api_key_id"]) || fromMessage(["api_key_id", "key"]),
    platform: fromPayload(["platform"]) || fromMessage(["platform"]),
    model: fromPayload(["model"]) || fromMessage(["model"]),
    method: fromPayload(["method"]) || fromMessage(["method"]),
    path: fromPayload(["path"]) || fromMessage(["path"]),
    ip: fromPayload(["ip", "client_ip"]) || fromMessage(["ip", "client_ip"]),
    protocol: fromPayload(["proto", "protocol"]) || fromMessage(["proto", "protocol"]),
    host: fromPayload(["host"]),
    level: fromPayload(["level"]).trim().toLowerCase(),
  };
}

function readableAccount(name: string, id: string): string {
  if (name && id) return `${name}（ID：${id}）`;
  if (name) return name;
  if (id) return `未知账号（ID：${id}）`;
  return "未记录";
}
