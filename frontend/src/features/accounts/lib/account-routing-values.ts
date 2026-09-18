import type { AccountStatus } from "@/api";

/** The upstream contract treats an omitted load factor as following concurrency. */
export function effectiveAccountLoadFactor(
  account: Pick<AccountStatus, "load_factor" | "concurrency">,
): string | null {
  const configured = account.load_factor?.trim();
  if (configured && Number.isFinite(Number(configured)) && Number(configured) > 0)
    return configured;
  if (account.concurrency == null) return null;
  return String(account.concurrency > 0 ? account.concurrency : 1);
}

export function effectiveTargetLoadFactor(
  account: Pick<
    AccountStatus,
    "target_load_factor" | "target_concurrency" | "load_factor" | "concurrency"
  >,
): string | null {
  return effectiveAccountLoadFactor({
    load_factor: account.target_load_factor ?? account.load_factor,
    concurrency: account.target_concurrency ?? account.concurrency,
  });
}

export function accountLoadFactorLabel(
  value: string | null,
  configured: string | null,
  concurrency: number | null,
): string {
  if (value == null) return "未读取";
  if (configured?.trim() && Number.isFinite(Number(configured)) && Number(configured) > 0)
    return value;
  if (concurrency != null && concurrency > 0) return `${value}（跟随并发）`;
  return `${value}（默认）`;
}
