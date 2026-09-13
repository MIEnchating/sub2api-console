import type { AccountStatus } from "@/api";

export const priorityOptions = ["manual", "automatic"] as const;
export type PriorityFilter = (typeof priorityOptions)[number];
export const priorityLabels: Record<PriorityFilter, string> = {
  manual: "人工优先",
  automatic: "自动调度",
};
export type AnimationFilters = {
  query: string;
  group: string | null;
  platform: string | null;
  priority: PriorityFilter | null;
};
export const defaultAnimationFilters: AnimationFilters = {
  query: "",
  group: null,
  platform: null,
  priority: null,
};

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
