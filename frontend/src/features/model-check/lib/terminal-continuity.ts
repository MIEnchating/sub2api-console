import type { Task, TerminalContinuityResult } from "@/api";
import { z } from "zod";

const verdictSchema = z.enum(["normal", "suspected", "inconclusive", "error"]);
const roundSchema = z.object({
  round: z.number().int().positive(),
  verdict: verdictSchema,
  request_id: z.string(),
  response: z.string().optional(),
  response_model: z.string().optional(),
  error: z.string().optional(),
  duration_ms: z.number().nonnegative(),
  completed_at: z.string(),
});
const resultSchema = roundSchema.omit({ round: true }).extend({
  account_id: z.string(),
  account_name: z.string(),
  model: z.string(),
  round_results: z.unknown().optional(),
});

export type TerminalHistoryResult = TerminalContinuityResult & { invalidRounds?: boolean };

const activeStatuses = new Set<Task["status"]>(["queued", "running", "waiting_input"]);

export function terminalContinuityResults(tasks: Task[]): Map<string, TerminalHistoryResult> {
  const result = new Map<string, TerminalHistoryResult>();
  const ordered = [...tasks].sort(
    (left, right) => Date.parse(right.created_at) - Date.parse(left.created_at),
  );
  for (const task of ordered) {
    const checks = Array.isArray(task.result.checks) ? task.result.checks : [];
    for (const value of checks) {
      const parsed = resultSchema.safeParse(value);
      if (!parsed.success || result.has(parsed.data.account_id)) continue;
      const rawRounds = parsed.data.round_results;
      const rounds: NonNullable<TerminalContinuityResult["round_results"]> = [];
      let invalidRounds = rawRounds !== undefined && !Array.isArray(rawRounds);
      for (const raw of Array.isArray(rawRounds) ? rawRounds : []) {
        const round = roundSchema.safeParse(raw);
        if (round.success) rounds.push(round.data);
        else invalidRounds = true;
      }
      result.set(parsed.data.account_id, {
        ...parsed.data,
        round_results: rawRounds === undefined ? undefined : rounds,
        invalidRounds,
      });
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
