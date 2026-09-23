import type { Task, TerminalContinuityResult } from "@/api";

const activeStatuses = new Set<Task["status"]>(["queued", "running", "waiting_input"]);

export function terminalContinuityResults(tasks: Task[]): Map<string, TerminalContinuityResult> {
  const result = new Map<string, TerminalContinuityResult>();
  const ordered = [...tasks].sort(
    (left, right) => Date.parse(right.created_at) - Date.parse(left.created_at),
  );
  for (const task of ordered) {
    const checks = Array.isArray(task.result.checks) ? task.result.checks : [];
    for (const value of checks) {
      if (!isTerminalContinuityResult(value) || result.has(value.account_id)) continue;
      result.set(value.account_id, value);
    }
  }
  return result;
}

export function terminalContinuityBusyIDs(tasks: Task[]): Set<string> {
  const result = new Set<string>();
  for (const task of tasks) {
    if (!activeStatuses.has(task.status) || !Array.isArray(task.result.account_ids)) continue;
    for (const id of task.result.account_ids) if (typeof id === "string") result.add(id);
  }
  return result;
}

export function isTerminalContinuityActive(task?: Task): boolean {
  return Boolean(task && activeStatuses.has(task.status));
}

function isTerminalContinuityResult(value: unknown): value is TerminalContinuityResult {
  if (!value || typeof value !== "object") return false;
  const row = value as Partial<TerminalContinuityResult>;
  return (
    typeof row.account_id === "string" &&
    typeof row.account_name === "string" &&
    typeof row.model === "string" &&
    typeof row.request_id === "string" &&
    ["normal", "suspected", "inconclusive", "error"].includes(row.verdict ?? "") &&
    typeof row.duration_ms === "number" &&
    typeof row.completed_at === "string"
  );
}
