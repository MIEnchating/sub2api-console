import type { QueryClient } from "@tanstack/react-query";

import type { AccountStatus, Task } from "../../../api";

function stableAccountID(value: unknown): string | null {
  const accountID = typeof value === "string" ? value.trim() : "";
  return /^[1-9]\d*$/.test(accountID) ? accountID : null;
}

export function accountIDsDeletedByTask(task?: Task): string[] {
  if (!task) return [];
  if (task.operation === "account-delete") {
    const accountID = stableAccountID(task.result.account_id);
    return task.result.local_projection_deleted === true && accountID ? [accountID] : [];
  }
  if (task.operation !== "account-delete-batch" || !Array.isArray(task.result.items)) return [];
  const accountIDs = new Set<string>();
  for (const raw of task.result.items) {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) continue;
    const item = raw as Record<string, unknown>;
    const accountID = stableAccountID(item.account_id);
    if (item.local_projection_deleted === true && accountID) accountIDs.add(accountID);
  }
  return [...accountIDs];
}

export function removeAccountsByID(
  accounts: AccountStatus[] | undefined,
  accountIDs: readonly string[],
): AccountStatus[] | undefined {
  if (!accounts || accountIDs.length === 0) return accounts;
  const removed = new Set(accountIDs);
  const next = accounts.filter((account) => !removed.has(account.id));
  return next.length === accounts.length ? accounts : next;
}

export function applyAccountDeletionProgress(queryClient: QueryClient, task?: Task): string[] {
  const accountIDs = accountIDsDeletedByTask(task);
  if (accountIDs.length === 0) return accountIDs;
  queryClient.setQueryData<AccountStatus[]>(["accounts"], (current) =>
    removeAccountsByID(current, accountIDs),
  );
  return accountIDs;
}
