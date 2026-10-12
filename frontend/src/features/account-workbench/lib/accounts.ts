import type { AccountFilter, WorkbenchAccount } from "../types";

export function accountStatus(account: WorkbenchAccount): string {
  const status = account.status.toLowerCase();
  if (["rate_limited", "overloaded", "temp_unschedulable", "expired"].includes(status))
    return status;
  if (!account.schedulable || ["inactive", "disabled", "manually_disabled"].includes(status))
    return "inactive";
  if (status === "active") return "active";
  return !status || status === "unknown" ? "unknown" : "error";
}
type AccountSearchEntry = {
  account: WorkbenchAccount;
  words: string;
  status: string;
};

export function createAccountSearchIndex(
  accounts: readonly WorkbenchAccount[],
): AccountSearchEntry[] {
  return accounts.map((account) => ({
    account,
    status: accountStatus(account),
    words: [account.name, account.email, account.id, ...account.groups.map((group) => group.name)]
      .join(" ")
      .toLowerCase(),
  }));
}

export function filterAccountSearchIndex(
  index: readonly AccountSearchEntry[],
  filter: AccountFilter,
): WorkbenchAccount[] {
  const query = filter.query.trim().toLowerCase();
  const matches: WorkbenchAccount[] = [];
  for (const entry of index) {
    if (filter.status !== "all" && entry.status !== filter.status) continue;
    if (filter.group !== "all" && !entry.account.groups.some((group) => group.id === filter.group))
      continue;
    if (query && !entry.words.includes(query)) continue;
    matches.push(entry.account);
  }
  return matches;
}
