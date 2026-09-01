export type DictionaryOption<T extends string = string> = Readonly<{
  value: T;
  label: string;
}>;

export type StatusTone = "success" | "warning" | "danger" | "info" | "neutral";

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
  return accountTypeAliases[normalized] ?? normalized;
}

export function accountTypeLabel(value: string | null | undefined): string | null {
  const raw = value?.trim();
  const normalized = accountTypeValue(raw);
  if (!normalized) return null;
  return accountTypeByValue.get(normalized)?.label ?? `未知类型（${raw}）`;
}
