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
          answer_truncated: z.boolean().optional(),
          error: z.string().optional(),
          request_id: z.string(),
        }),
      ),
    })
    .optional(),
  account_id: z.string(),
  account_name: z.string(),
  endpoint: z.string().optional(),
  platform: z.enum(["openai", "anthropic"]).optional(),
  model: z.string(),
  response_model: z.string().optional(),
  request_id: z.string(),
  status: z.enum(["succeeded", "failed"]),
  phase: z.enum(["queued", "running", "generating", "completed"]).optional(),
  html: z.string().optional(),
  source: z.string().optional(),
  prompt: z.string().optional(),
  reasoning_effort: z.string().optional(),
  usage: z
    .object({
      input_tokens: z.number().int().nonnegative().optional(),
      output_tokens: z.number().int().nonnegative().optional(),
      total_tokens: z.number().int().nonnegative().optional(),
      reasoning_tokens: z.number().int().nonnegative().optional(),
    })
    .optional(),
  generation_duration_ms: z.number().nonnegative().optional(),
  svg: z.string().optional(),
  error: z.string().optional(),
  retry_count: z.number().int().nonnegative().optional(),
  duration_ms: z.number().nonnegative(),
  completed_at: z.string().refine((value) => Number.isFinite(Date.parse(value))),
});

const animationTargetSchema = animationResultSchema.pick({
  account_id: true,
  model: true,
  endpoint: true,
  platform: true,
});

function animationTaskResults(task: Task, precheck: boolean): Map<string, AnimationResult> {
  const results = new Map<string, AnimationResult>();
  if (!Array.isArray(task?.result.animations)) return results;
  for (const item of task.result.animations) {
    const parsed = animationResultSchema.safeParse(item);
    if (!parsed.success) continue;
    const result: AnimationResult = { ...parsed.data };
    if (result.phase && result.phase !== "completed") continue;
    const isPrecheck =
      result.mode === "precheck" ||
      (!result.mode &&
        (task.operation === "account-model-precheck" || task.result.mode === "precheck"));
    if (isPrecheck !== precheck) continue;
    if (!isPrecheck) result.source_task_id = task.id;
    const existing = results.get(result.account_id);
    if (!existing || Date.parse(result.completed_at) >= Date.parse(existing.completed_at))
      results.set(result.account_id, result);
  }
  return results;
}

function animationTaskPhases(task: Task): Map<string, AnimationResult["phase"]> {
  const phases = new Map<string, AnimationResult["phase"]>();
  if (!Array.isArray(task?.result.animations)) return phases;
  for (const item of task.result.animations) {
    const parsed = animationResultSchema.safeParse(item);
    if (parsed.success && parsed.data.phase && parsed.data.phase !== "completed") {
      phases.set(parsed.data.account_id, parsed.data.phase);
    }
  }
  return phases;
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
  status: "starting" | "queued" | "running" | "generating" | "waiting_input";
  taskID?: string;
  batchSize?: number;
  completed?: number;
};

export function animationActivityLabel(status: AnimationActivity["status"]): string {
  switch (status) {
    case "starting":
      return "正在启动检测";
    case "queued":
      return "排队中，等待并发槽位";
    case "running":
      return "已开始请求，等待首字";
    case "generating":
      return "已收到首字，生成中";
    case "waiting_input":
      return "等待检测输入";
  }
}

function animationActivityStatus(
  phase: AnimationResult["phase"] | undefined,
  taskStatus: Task["status"],
): AnimationActivity["status"] {
  if (phase === "queued") return "queued";
  if (phase === "generating") return "generating";
  if (phase === "running") return "running";
  if (taskStatus === "waiting_input") return "waiting_input";
  return taskStatus === "queued" ? "queued" : "running";
}

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
  const targets = new Map<string, z.infer<typeof animationTargetSchema>>();
  const busyIDs = new Set(submitting);
  const activeTaskIDs = new Set<string>();
  const sorted = tasks
    .filter((task): task is Task => task !== undefined)
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
  for (const task of sorted) {
    if (Array.isArray(task.result.targets)) {
      for (const target of task.result.targets) {
        const parsed = animationTargetSchema.safeParse(target);
        if (parsed.success) targets.set(parsed.data.account_id, parsed.data);
      }
    }
    const precheck = task.operation === "account-model-precheck" || task.result.mode === "precheck";
    const combined = task.operation === "account-model-combined" || task.result.mode === "both";
    const taskPrechecks = animationTaskResults(task, true);
    const taskAnimations = animationTaskResults(task, false);
    const taskPhases = animationTaskPhases(task);
    const ids = animationTaskAccountIDs(task, new Map([...taskPrechecks, ...taskAnimations]));
    let modes = combined ? [true, false] : [precheck];
    const configuration = task.result.configuration;
    if (
      task.operation === "managed-model-detection" &&
      configuration &&
      typeof configuration === "object"
    ) {
      const config = configuration as Record<string, unknown>;
      modes = [];
      if (config.precheck === true) modes.push(true);
      if (config.animation !== false) modes.push(false);
    }
    const taskResults = modes.includes(false) ? taskAnimations : taskPrechecks;
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
      for (const id of ids) statusMap.set(id, phaseResults.get(id)?.status ?? task.status);
    }
    if (task.status !== "queued" && task.status !== "running" && task.status !== "waiting_input")
      continue;
    activeTaskIDs.add(task.id);
    for (const id of ids) {
      // The backend reserves the batch's accounts until the entire task finishes.
      busyIDs.add(id);
      if (modes.length > 0 && !taskResults.has(id))
        activities.set(id, {
          ...(modes.includes(true) && (!modes.includes(false) || !taskPrechecks.has(id))
            ? { mode: "precheck" as const }
            : {}),
          status: animationActivityStatus(taskPhases.get(id), task.status),
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
    targets,
    results,
    statuses,
    activities,
    busyIDs,
    activeTaskIDs,
    precheckResults,
    precheckStatuses,
  };
}
