import type { WorkbenchAccount } from "../types";

export function collectAccountGroups(
  accounts: readonly WorkbenchAccount[],
): Map<string, WorkbenchAccount["groups"][number]> {
  const groups = new Map<string, WorkbenchAccount["groups"][number]>();
  for (const account of accounts) {
    for (const group of account.groups) groups.set(group.id, group);
  }
  return groups;
}
