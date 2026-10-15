import type { AccountStatus } from "@/api";
import type { AnimationAccountSort } from "../constants";
import { supportsAnimationPlatform } from "../constants";

export const priorityOptions = ["manual", "automatic"] as const;
export type PriorityFilter = (typeof priorityOptions)[number];
export const priorityLabels: Record<PriorityFilter, string> = {
  manual: "手动控制",
  automatic: "自动调度",
};
export type AnimationFilters = {
  sort: AnimationAccountSort;
  query: string;
  group: string | null;
  platform: string | null;
  priority: PriorityFilter | null;
};
export const defaultAnimationFilters: AnimationFilters = {
  sort: "default",
  query: "",
  group: null,
  platform: null,
  priority: null,
};

export function selectableDetectionAccountIDs(
  accounts: AccountStatus[],
  busyIDs: Set<string>,
): string[] {
  return accounts
    .filter(
      (account) =>
        !account.upstream_excluded &&
        !busyIDs.has(account.id) &&
        (account.platform == null || supportsAnimationPlatform(account.platform)),
    )
    .map((account) => account.id);
}

export function filterAnimationAccounts(
  accounts: AccountStatus[],
  filters: AnimationFilters,
): AccountStatus[] {
  const query = filters.query.trim().toLocaleLowerCase();
  return accounts.filter((account) => {
    if (filters.group !== null && !account.groups.includes(filters.group)) return false;
    if (filters.platform !== null && account.platform !== filters.platform) return false;
    if (filters.priority === "manual" && account.manual_priority == null) return false;
    if (filters.priority === "automatic" && account.manual_priority != null) return false;
    return [
      account.id,
      account.name,
      account.platform,
      account.upstream_host,
      ...account.groups,
    ].some((value) => value?.toLocaleLowerCase().includes(query));
  });
}
