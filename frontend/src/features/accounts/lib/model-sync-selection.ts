import type { AccountModelSyncAccount } from "@/api";
import type { ModelSyncPlatformGroup } from "./model-sync-groups";

export function includedGroupModelKeys(
  accounts: readonly AccountModelSyncAccount[],
  group: ModelSyncPlatformGroup["groups"][number] | undefined,
  exclusions?: ReadonlySet<string>,
  inclusions?: ReadonlySet<string>,
): Set<string> {
  const selected = new Set<string>();
  if (!group) return selected;
  const accountIds = new Set(group.accountIds);
  const enabled = new Set<string>();
  for (const account of accounts) {
    if (!accountIds.has(account.account_id)) continue;
    for (const model of account.enabled_models ?? []) enabled.add(model.toLocaleLowerCase());
  }
  for (const item of group.models) {
    const key = item.model.toLocaleLowerCase();
    if (exclusions?.has(key)) continue;
    if (inclusions?.has(key) || enabled.has(key)) selected.add(key);
  }
  return selected;
}
