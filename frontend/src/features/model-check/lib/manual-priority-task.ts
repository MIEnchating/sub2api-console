import type { QueryClient } from "@tanstack/react-query";
import { z } from "zod";

import type { AccountStatus, Task } from "@/api";

const manualPriorityResultSchema = z.object({
  account_id: z.string(),
  readback_confirmed: z.literal(true),
  manual_priority: z.number().int().positive().nullable(),
  sync_balance_multiplier: z.boolean().optional(),
});

const readbackFieldsSchema = z.object({
  priority: z.number().int().nullable().optional(),
  load_factor: z
    .union([z.string(), z.number()])
    .nullable()
    .transform((value) => (value === null ? null : String(value)))
    .optional(),
  concurrency: z.number().int().nullable().optional(),
  schedulable: z.boolean().nullable().optional(),
});

export async function applyManualPriorityTaskResult(
  client: QueryClient,
  task: Task,
  accountID: string,
): Promise<void> {
  if (task.status !== "succeeded") return;
  const parsed = manualPriorityResultSchema.safeParse(task.result);
  if (!parsed.success || parsed.data.account_id !== accountID) return;

  // Apply only confirmed server values; never infer success from the submitted form.
  const patch: Partial<AccountStatus> = { manual_priority: parsed.data.manual_priority };
  if (parsed.data.manual_priority === null) {
    patch.manual_sync_balance_multiplier = false;
  } else if (parsed.data.sync_balance_multiplier !== undefined) {
    patch.manual_sync_balance_multiplier = parsed.data.sync_balance_multiplier;
  }
  const fields = readbackFieldsSchema.safeParse(task.result.after);
  if (fields.success) Object.assign(patch, fields.data);

  // Discard requests started before the write, so their late responses cannot revert it.
  await Promise.all([
    client.cancelQueries({ queryKey: ["accounts"], exact: true }),
    client.cancelQueries({ queryKey: ["account-detail", accountID] }),
  ]);
  client.setQueryData<AccountStatus[]>(["accounts"], (accounts) =>
    accounts?.map((account) => (account.id === accountID ? { ...account, ...patch } : account)),
  );
  client.setQueryData<AccountStatus>(["account-detail", accountID], (account) =>
    account ? { ...account, ...patch } : account,
  );
}
