import type { AnimationResult, Task } from "@/api";
import { z } from "zod";

const animationResultSchema = z.object({
  mode: z.enum(["animation", "precheck"]).optional(),
  precheck: z
    .object({
      verdict: z.enum(["passed", "not_passed", "inconclusive", "error"]),
      profile_version: z.string(),
      questions: z.array(
        z.object({
          id: z.string(),
          verdict: z.enum(["passed", "not_passed", "inconclusive", "error"]),
          answer: z.string().optional(),
          error: z.string().optional(),
          request_id: z.string(),
        }),
      ),
    })
    .optional(),
  account_id: z.string(),
  account_name: z.string(),
  model: z.string(),
  response_model: z.string().optional(),
  request_id: z.string(),
  status: z.enum(["succeeded", "failed"]),
  svg: z.string().optional(),
  error: z.string().optional(),
  retry_count: z.number().int().nonnegative().optional(),
  duration_ms: z.number().nonnegative(),
  completed_at: z.string().refine((value) => Number.isFinite(Date.parse(value))),
});

function animationTaskResults(task: Task, precheck: boolean): Map<string, AnimationResult> {
  const results = new Map<string, AnimationResult>();
  if (!Array.isArray(task?.result.animations)) return results;
  for (const item of task.result.animations) {
    const parsed = animationResultSchema.safeParse(item);
    if (!parsed.success) continue;
    const result = parsed.data;
    const isPrecheck =
      result.mode === "precheck" ||
      (!result.mode &&
        (task.operation === "account-model-precheck" || task.result.mode === "precheck"));
    if (isPrecheck !== precheck) continue;
    const existing = results.get(result.account_id);
    if (!existing || Date.parse(result.completed_at) >= Date.parse(existing.completed_at))
      results.set(result.account_id, result);
  }
  return results;
}

function animationTaskAccountIDs(task: Task, results: Map<string, AnimationResult>): Set<string> {
  const ids = new Set(results.keys());
  if (Array.isArray(task?.result.account_ids)) {
    for (const id of task.result.account_ids) if (typeof id === "string") ids.add(id);
  }
  if (Array.isArray(task?.result.targets)) {
    for (const target of task.result.targets)
      if (typeof target === "object" && target !== null && typeof target.account_id === "string")
        ids.add(target.account_id);
  }
  return ids;
}

export type AnimationActivity = {
  mode?: "precheck";
  status: "starting" | "queued" | "running" | "waiting_input";
  taskID?: string;
  batchSize?: number;
  completed?: number;
};

export function collectAnimationTasks(
  tasks: (Task | undefined)[],
  submitting: Set<string>,
  precheckSubmitting?: Set<string>,
) {
  const results = new Map<string, AnimationResult>();
  const precheckResults = new Map<string, AnimationResult>();
  const precheckStatuses = new Map<string, Task["status"]>();
  const statuses = new Map<string, Task["status"]>();
  const activities = new Map<string, AnimationActivity>();
  const busyIDs = new Set(submitting);
  const activeTaskIDs = new Set<string>();
  const sorted = tasks
    .filter((task): task is Task => task !== undefined)
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
  for (const task of sorted) {
    const precheck = task.operation === "account-model-precheck" || task.result.mode === "precheck";
    const combined = task.operation === "account-model-combined" || task.result.mode === "both";
    const taskPrechecks = animationTaskResults(task, true);
    const taskAnimations = animationTaskResults(task, false);
    const taskResults = precheck ? taskPrechecks : taskAnimations;
    const ids = animationTaskAccountIDs(task, new Map([...taskPrechecks, ...taskAnimations]));
    const modes = combined ? [true, false] : [precheck];
    for (const isPrecheck of modes) {
      const resultMap = isPrecheck ? precheckResults : results;
      const statusMap = isPrecheck ? precheckStatuses : statuses;
      const phaseResults = isPrecheck ? taskPrechecks : taskAnimations;
      if (isPrecheck) {
        for (const id of ids) resultMap.delete(id);
      }
      for (const [id, result] of phaseResults) {
        const existing = resultMap.get(id);
        if (!existing || Date.parse(result.completed_at) >= Date.parse(existing.completed_at))
          resultMap.set(id, result);
      }
      for (const id of ids)
        statusMap.set(id, combined ? (phaseResults.get(id)?.status ?? task.status) : task.status);
    }
    if (task.status !== "queued" && task.status !== "running" && task.status !== "waiting_input")
      continue;
    activeTaskIDs.add(task.id);
    for (const id of ids) {
      // The backend reserves the batch's accounts until the entire task finishes.
      busyIDs.add(id);
      if (!taskResults.has(id))
        activities.set(id, {
          ...(precheck || (combined && !taskPrechecks.has(id))
            ? { mode: "precheck" as const }
            : {}),
          status: task.status,
          taskID: task.id,
          batchSize: ids.size,
          completed: taskResults.size,
        });
    }
  }
  for (const id of submitting)
    activities.set(id, {
      status: "starting",
      ...(precheckSubmitting?.has(id) ? { mode: "precheck" as const } : {}),
    });
  return {
    results,
    statuses,
    activities,
    busyIDs,
    activeTaskIDs,
    precheckResults,
    precheckStatuses,
  };
}
