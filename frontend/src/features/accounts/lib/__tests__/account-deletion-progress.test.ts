import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import type { AccountStatus, Task } from "../../../../api";
import {
  accountIDsDeletedByTask,
  applyAccountDeletionProgress,
  removeAccountsByID,
} from "../account-deletion-progress";

function task(operation: string, result: Record<string, unknown>): Task {
  return {
    id: "delete-task",
    skill: "sub2api-account-management",
    operation,
    status: "running",
    progress: 50,
    message: "正在删除账号",
    result,
    created_at: "2026-09-02T00:00:00Z",
    updated_at: "2026-09-02T00:00:01Z",
  };
}

function account(id: string): AccountStatus {
  return { id, name: `账号 ${id}` } as AccountStatus;
}

describe("account deletion progress", () => {
  it("removes a single account only after its local projection is confirmed deleted", () => {
    expect(
      accountIDsDeletedByTask(
        task("account-delete", { account_id: "37", local_projection_deleted: true }),
      ),
    ).toEqual(["37"]);
    expect(
      accountIDsDeletedByTask(
        task("account-delete", { account_id: "37", local_projection_deleted: false }),
      ),
    ).toEqual([]);
  });

  it("removes successful batch items while the remaining accounts are still running", () => {
    const deleted = accountIDsDeletedByTask(
      task("account-delete-batch", {
        items: [
          { account_id: "37", status: "succeeded", local_projection_deleted: true },
          { account_id: "38", status: "failed", local_projection_deleted: false },
        ],
      }),
    );

    expect(deleted).toEqual(["37"]);
    expect(removeAccountsByID([account("37"), account("38")], deleted)).toEqual([account("38")]);
  });

  it("preserves the existing cache reference when no listed account was deleted", () => {
    const accounts = [account("38")];

    expect(removeAccountsByID(accounts, ["37"])).toBe(accounts);
    expect(removeAccountsByID(undefined, ["37"])).toBeUndefined();
  });

  it("updates the live React Query account cache from partial batch progress", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [account("37"), account("38")]);

    const deleted = applyAccountDeletionProgress(
      queryClient,
      task("account-delete-batch", {
        items: [{ account_id: "37", status: "succeeded", local_projection_deleted: true }],
      }),
    );

    expect(deleted).toEqual(["37"]);
    expect(queryClient.getQueryData(["accounts"])).toEqual([account("38")]);
    queryClient.clear();
  });
});
