import type { AccountStatus, GroupStatus } from "@/api";
import { accountTypeLabel, accountTypeOptions, accountTypeValue } from "@/lib/domain-dictionaries";

export { accountTypeLabel, accountTypeOptions, accountTypeValue };

const platformLabels: Record<string, string> = {
  anthropic: "Anthropic",
  openai: "OpenAI",
  gemini: "Gemini",
  antigravity: "Antigravity",
  grok: "Grok",
  kimi: "Kimi",
  zhipu: "Zhipu GLM",
  deepseek: "DeepSeek",
  composite: "Composite",
  claude: "Claude",
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
