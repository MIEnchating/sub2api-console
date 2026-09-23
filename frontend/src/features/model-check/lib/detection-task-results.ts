import type { AnimationResult, GroupStatus, Task, TerminalContinuityResult } from "@/api";
import { collectAnimationTasks } from "./animation-task-results";
export type AccountRow = {
  id: string;
  name: string;
  animation?: AnimationResult;
  precheck?: AnimationResult;
  terminal?: TerminalContinuityResult;
};
type GroupRows = { id: string; name: string; rows: AccountRow[] };

export function detectionAccountRows(
  task: Task | undefined,
  state: ReturnType<typeof collectAnimationTasks>,
  terminals: Map<string, TerminalContinuityResult>,
): Map<string, AccountRow> {
  const rows = new Map<string, AccountRow>();
  const add = (id: string, name?: string): AccountRow => {
    const row = rows.get(id) ?? { id, name: name || `账号 ${id}` };
    if (name) row.name = name;
    rows.set(id, row);
    return row;
  };
  const names = isRecord(task?.result.account_names_by_id) ? task.result.account_names_by_id : {};
  for (const id of stringArray(task?.result.account_ids))
    add(id, typeof names[id] === "string" ? names[id] : undefined);
  for (const target of Array.isArray(task?.result.targets) ? task.result.targets : [])
    if (isRecord(target) && typeof target.account_id === "string") add(target.account_id);
  for (const [id, value] of state.results)
    Object.assign(add(id, value.account_name), { animation: value });
  for (const [id, value] of state.precheckResults)
    Object.assign(add(id, value.account_name), { precheck: value });
  for (const value of terminals.values())
    Object.assign(add(value.account_id, value.account_name), { terminal: value });
  return rows;
}

export function detectionResultGroups(
  task: Task | undefined,
  groups: GroupStatus[] | undefined,
  rows: Map<string, AccountRow>,
): GroupRows[] {
  const config = isRecord(task?.result.configuration) ? task?.result.configuration : undefined;
  const selected = stringArray(config?.group_ids);
  const memberships = isRecord(task?.result.group_ids_by_account)
    ? task?.result.group_ids_by_account
    : {};
  const names = isRecord(task?.result.group_names_by_id) ? task.result.group_names_by_id : {};
  const output: GroupRows[] = [...new Set(selected)].map((id) => ({
    id,
    name:
      typeof names[id] === "string"
        ? names[id]
        : (groups?.find((group) => group.id === id)?.name ?? "分组 " + id),
    rows: [],
  }));
  const ungrouped: GroupRows = { id: "__ungrouped", name: "分组未记录", rows: [] };
  for (const row of rows.values()) {
    const ids = stringArray(memberships[row.id]);
    const target = output.find((group) => ids.includes(group.id));
    if (target) target.rows.push(row);
    else if (!("group_ids_by_account" in (task?.result ?? {})) && output.length === 1)
      output[0].rows.push(row);
    else ungrouped.rows.push(row);
  }
  if (ungrouped.rows.length > 0) output.push(ungrouped);
  return output.filter((group) => group.rows.length > 0 || group.id !== "__ungrouped");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}
