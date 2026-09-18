import type { AccountFilter, WorkbenchAccount } from "../types";

export function accountStatus(account: WorkbenchAccount): string {
  const status = account.status.toLowerCase();
  if (!account.schedulable || ["inactive", "disabled", "manually_disabled"].includes(status))
    return "inactive";
  if (status === "active") return "active";
  return !status || status === "unknown" ? "unknown" : "error";
}
export function matchesAccount(account: WorkbenchAccount, filter: AccountFilter): boolean {
  const words = [
    account.name,
    account.email,
    account.id,
    ...account.groups.map((group) => group.name),
  ]
    .join(" ")
    .toLowerCase();
  return (
    words.includes(filter.query.trim().toLowerCase()) &&
    (filter.status === "all" || accountStatus(account) === filter.status) &&
    (filter.group === "all" || account.groups.some((group) => group.id === filter.group))
  );
}
