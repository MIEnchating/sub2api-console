import type { AccountStatus, GroupStatus } from "@/api";

const platformLabels: Record<string, string> = {
  openai: "OpenAI",
  claude: "Claude",
  anthropic: "Anthropic",
  grok: "Grok",
  gemini: "Gemini",
};

const accountTypeLabels: Record<string, string> = {
  apikey: "API Key",
  api_key: "API Key",
  "api-key": "API Key",
  oauth: "OAuth",
  sub2api: "Sub2API",
  newapi: "New API",
  oneapi: "OneAPI",
};

function mappedLabel(
  value: string | null | undefined,
  labels: Record<string, string>,
): string | null {
  const normalized = value?.trim();
  if (!normalized) return null;
  return labels[normalized.toLowerCase()] ?? normalized;
}

function accountPlatformLabel(value: string | null | undefined): string | null {
  return mappedLabel(value, platformLabels);
}

export function accountTypeLabel(value: string | null | undefined): string | null {
  return mappedLabel(value, accountTypeLabels);
}

export function accountIdentityMeta(account: AccountStatus): string {
  const values = [
    `#${account.id}`,
    accountPlatformLabel(account.platform),
    accountTypeLabel(account.account_type ?? account.upstream_type),
  ].filter((value): value is string => Boolean(value));
  return [...new Set(values)].join(" · ");
}

export function groupPlatformSummary(group: GroupStatus): string {
  return accountPlatformLabel(group.platform) ?? "未识别";
}
