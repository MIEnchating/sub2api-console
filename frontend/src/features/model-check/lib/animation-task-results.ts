import type { AnimationResult, Task } from "@/api";
import { z } from "zod";

const animationResultSchema = z.object({
  account_id: z.string(),
  account_name: z.string(),
  model: z.string(),
  response_model: z.string().optional(),
  request_id: z.string(),
  status: z.enum(["succeeded", "failed"]),
  svg: z.string().optional(),
  error: z.string().optional(),
  duration_ms: z.number().nonnegative(),
  completed_at: z.string().refine((value) => Number.isFinite(Date.parse(value))),
});

function animationTaskResults(task?: Task): Map<string, AnimationResult> {
  const results = new Map<string, AnimationResult>();
  if (!Array.isArray(task?.result.animations)) return results;
  for (const item of task.result.animations) {
    const parsed = animationResultSchema.safeParse(item);
    if (!parsed.success) continue;
    const result = parsed.data;
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
  status: "starting" | "queued" | "running" | "waiting_input";
  taskID?: string;
  batchSize?: number;
  completed?: number;
};

export function collectAnimationTasks(tasks: (Task | undefined)[], submitting: Set<string>) {
  const results = new Map<string, AnimationResult>();
  const statuses = new Map<string, Task["status"]>();
  const activities = new Map<string, AnimationActivity>();
  const busyIDs = new Set(submitting);
  const activeTaskIDs = new Set<string>();
  const sorted = tasks
    .filter((task): task is Task => task !== undefined)
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
  for (const task of sorted) {
    const taskResults = animationTaskResults(task);
    const ids = animationTaskAccountIDs(task, taskResults);
    for (const [id, result] of taskResults) {
      const existing = results.get(id);
      if (!existing || Date.parse(result.completed_at) >= Date.parse(existing.completed_at))
        results.set(id, result);
    }
    for (const id of ids) statuses.set(id, task.status);
    if (task.status !== "queued" && task.status !== "running" && task.status !== "waiting_input")
      continue;
    activeTaskIDs.add(task.id);
    for (const id of ids) {
      // The backend reserves the batch's accounts until the entire task finishes.
      busyIDs.add(id);
      if (!taskResults.has(id))
        activities.set(id, {
          status: task.status,
          taskID: task.id,
          batchSize: ids.size,
          completed: taskResults.size,
        });
    }
  }
  for (const id of submitting) activities.set(id, { status: "starting" });
  return { results, statuses, activities, busyIDs, activeTaskIDs };
}
