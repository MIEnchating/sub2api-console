import type { AnimationResult, Task } from "@/api";
import { compareTimestamps } from "@/lib/time-order";
import { z } from "zod";
import { animationPlatforms } from "../constants";

export const animationResultSchema = z.object({
  quality: z
    .object({
      rubric_version: z.string().optional(),
      basis: z
        .array(z.object({ requirement: z.string(), evidence: z.string(), assessment: z.string() }))
        .optional(),
      issues: z
        .array(
          z.object({
            severity: z.enum(["major", "minor"]),
            scope: z.enum(["subject", "motion", "rendering", "secondary"]),
            evidence: z.string(),
            impact: z.string(),
          }),
        )
        .optional(),
      source: z.literal("model_review").optional(),
      explanation: z.string().optional(),
      reviewer_account_id: z.string().optional(),
      settings_version: z.number().optional(),
      reviewed_at: z.string().optional(),
      response_model: z.string().optional(),
      status: z.enum(["normal", "suspicious", "unknown"]),
      score: z.number().min(0).max(1).nullable(),
      reason: z.string().optional(),
      benchmark: z.string(),
      model_version: z.string().optional(),
      model_sha256: z.string().optional(),
    })
    .optional(),
  quality_prediction_sha256: z.string().optional(),
  quality_annotation: z
    .object({
      source: z.enum(["manual", "confirmed_auto"]).optional(),
      prediction_sha256: z.string().optional(),
      label: z.enum(["normal", "suspicious", "unknown"]),
      reason: z.string(),
      source_group: z.string(),
      version: z.number(),
      updated_at: z.string(),
      updated_by: z.string(),
    })
    .optional(),
  manual_verdict: z
    .object({
      verdict: z.enum(["passed", "not_passed", "unmarked"]),
      updated_at: z.string(),
      updated_by: z.string(),
    })
    .optional(),
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
  platform: z.enum(animationPlatforms).optional(),
  model: z.string(),
  response_model: z.string().optional(),
  request_id: z.string(),
  status: z.enum(["succeeded", "failed"]),
  phase: z.enum(["queued", "running", "generating", "reviewing", "completed"]).optional(),
  html: z.string().optional(),
  source: z.string().optional(),
  source_truncated: z.boolean().optional(),
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

const animationTargetSchema = animationResultSchema
  .pick({
    account_id: true,
    model: true,
    endpoint: true,
    platform: true,
  })
  .extend({ models: z.array(z.string()).optional() });

const terminalTaskStatuses = new Set<Task["status"]>([
  "succeeded",
  "partial",
  "failed",
  "cancelled",
]);

function animationTaskRows(
  task: Task,
  accountID?: string,
): {
  prechecks: Map<string, AnimationResult>;
  animations: Map<string, AnimationResult>;
  phases: Map<string, AnimationResult["phase"]>;
  modelAnimations: Map<string, AnimationResult>;
  modelPrechecks: Map<string, AnimationResult>;
} {
  const prechecks = new Map<string, AnimationResult>();
  const animations = new Map<string, AnimationResult>();
  const modelAnimations = new Map<string, AnimationResult>();
  const modelPrechecks = new Map<string, AnimationResult>();
  const phases = new Map<string, AnimationResult["phase"]>();
  if (!Array.isArray(task.result.animations))
    return { prechecks, animations, phases, modelAnimations, modelPrechecks };
  for (const item of task.result.animations) {
    // Single-account consumers do not need to validate or copy the other rows.
    if (
      accountID !== undefined &&
      (typeof item !== "object" || item === null || item.account_id !== accountID)
    )
      continue;
    const parsed = animationResultSchema.safeParse(item);
    if (!parsed.success) continue;
    const result: AnimationResult = parsed.data;
    if (result.phase && result.phase !== "completed") phases.set(result.account_id, result.phase);
    // Older managed tasks persisted their final rows with the last streaming
    // phase. A terminal task proves that the row is no longer in flight.
    const generated =
      result.phase === "reviewing" &&
      result.status === "succeeded" &&
      Boolean(result.html || result.svg);
    if (
      result.phase &&
      result.phase !== "completed" &&
      !generated &&
      !terminalTaskStatuses.has(task.status)
    )
      continue;
    const isPrecheck =
      result.mode === "precheck" ||
      (!result.mode &&
        (result.precheck !== undefined ||
          task.operation === "account-model-precheck" ||
          task.result.mode === "precheck"));
    const results = isPrecheck ? prechecks : animations;
    if (!isPrecheck) {
      result.source_task_id = task.id;
      result.source_task_status = task.status;
    }
    const modelMap = isPrecheck ? modelPrechecks : modelAnimations;
    const modelKey = JSON.stringify([result.account_id, result.model]);
    const previousModel = modelMap.get(modelKey);
    if (!previousModel || compareTimestamps(result.completed_at, previousModel.completed_at) >= 0)
      modelMap.set(modelKey, result);
    const existing = results.get(result.account_id);
    if (!existing || compareTimestamps(result.completed_at, existing.completed_at) >= 0)
      results.set(result.account_id, result);
  }
  return { prechecks, animations, phases, modelAnimations, modelPrechecks };
}

function animationTaskModes(task: Task): boolean[] {
  const configuration = task.result.configuration;
  if (
    task.operation === "managed-model-detection" &&
    configuration &&
    typeof configuration === "object"
  ) {
    const config = configuration as Record<string, unknown>;
    const modes: boolean[] = [];
    if (config.precheck === true) modes.push(true);
    if (config.animation !== false) modes.push(false);
    return modes;
  }
  if (task.operation === "account-model-combined" || task.result.mode === "both")
    return [true, false];
  return [task.operation === "account-model-precheck" || task.result.mode === "precheck"];
}

export type AnimationTaskSelection = {
  status: Task["status"];
  result: AnimationResult | undefined;
};

export function selectAnimationTaskResult(
  task: Task,
  accountID: string,
  requestID: string,
): AnimationTaskSelection {
  const result = animationTaskModes(task).includes(false)
    ? [...animationTaskRows(task, accountID).modelAnimations.values()].find(
        (item) => item.request_id === requestID,
      )
    : undefined;
  return {
    status: task.status,
    result: result?.request_id === requestID ? result : undefined,
  };
}

function animationTaskAccountIDs(
  task: Task,
  prechecks: Map<string, AnimationResult>,
  animations: Map<string, AnimationResult>,
): Set<string> {
  const ids = new Set(prechecks.keys());
  for (const id of animations.keys()) ids.add(id);
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
  status: "starting" | "queued" | "running" | "generating" | "reviewing" | "waiting_input";
  taskID?: string;
  batchSize?: number;
  completed?: number;
};

export function animationQualityPending(result: AnimationResult): boolean {
  return (
    result.phase === "reviewing" &&
    Boolean(result.source_task_status && !terminalTaskStatuses.has(result.source_task_status))
  );
}

export function animationWaitingForGeneration(
  result: AnimationResult,
  activity?: AnimationActivity,
): boolean {
  if (!activity) return false;
  // A different model may still be running in this task. Keep this model's
  // generated result visible, but never present a previous task as the new run.
  return (
    !activity.taskID ||
    result.source_task_id !== activity.taskID ||
    (result.phase !== "completed" && result.phase !== "reviewing" && result.phase !== undefined)
  );
}

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
    case "reviewing":
      return "动画已生成，正在自动标注质量";
    case "waiting_input":
      return "等待检测输入";
  }
}

function animationActivityStatus(
  phase: AnimationResult["phase"] | undefined,
  taskStatus: Task["status"],
): AnimationActivity["status"] {
  if (phase === "queued") return "queued";
  if (phase === "reviewing") return "reviewing";
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
  const allModelAnimations = new Map<string, AnimationResult>();
  const allModelPrechecks = new Map<string, AnimationResult>();
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
    .sort((a, b) => {
      const timeOrder = compareTimestamps(a.created_at, b.created_at);
      if (timeOrder !== 0) return timeOrder;
      if (a.id < b.id) return -1;
      if (a.id > b.id) return 1;
      return 0;
    });
  for (const task of sorted) {
    if (Array.isArray(task.result.targets)) {
      for (const target of task.result.targets) {
        const parsed = animationTargetSchema.safeParse(target);
        if (parsed.success) targets.set(parsed.data.account_id, parsed.data);
      }
    }
    const rows = animationTaskRows(task);
    for (const [source, output] of [
      [rows.modelAnimations, allModelAnimations],
      [rows.modelPrechecks, allModelPrechecks],
    ] as const) {
      for (const [key, result] of source) {
        const previous = output.get(key);
        if (!previous || compareTimestamps(result.completed_at, previous.completed_at) >= 0)
          output.set(key, result);
      }
    }
    const taskPrechecks = rows.prechecks;
    const taskAnimations = rows.animations;
    const ids = animationTaskAccountIDs(task, taskPrechecks, taskAnimations);
    const modes = animationTaskModes(task);
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
        if (!existing || compareTimestamps(result.completed_at, existing.completed_at) >= 0)
          resultMap.set(id, result);
      }
      for (const id of ids) statusMap.set(id, phaseResults.get(id)?.status ?? task.status);
    }
    if (task.status !== "queued" && task.status !== "running" && task.status !== "waiting_input")
      continue;
    activeTaskIDs.add(task.id);
    const completedAccounts = [...taskResults.values()].filter(
      (result) => result.phase !== "reviewing",
    ).length;
    for (const id of ids) {
      // The backend reserves the batch's accounts until the entire task finishes.
      busyIDs.add(id);
      const expectedModels = targets.get(id)?.models;
      const finishedModels = [
        ...(modes.includes(false) ? rows.modelAnimations : rows.modelPrechecks).values(),
      ].filter((result) => result.account_id === id && result.phase !== "reviewing").length;
      if (
        modes.length > 0 &&
        (!taskResults.has(id) || finishedModels < (expectedModels?.length ?? 1))
      )
        activities.set(id, {
          ...(modes.includes(true) && (!modes.includes(false) || !taskPrechecks.has(id))
            ? { mode: "precheck" as const }
            : {}),
          status: animationActivityStatus(rows.phases.get(id), task.status),
          taskID: task.id,
          batchSize: ids.size,
          completed: completedAccounts,
        });
    }
  }
  for (const id of submitting)
    activities.set(id, {
      status: "starting",
      ...(precheckSubmitting?.has(id) ? { mode: "precheck" as const } : {}),
    });
  const modelResults = new Map<string, AnimationResult[]>();
  const modelPrecheckResults = new Map<string, AnimationResult[]>();
  for (const [source, output] of [
    [allModelAnimations, modelResults],
    [allModelPrechecks, modelPrecheckResults],
  ] as const) {
    for (const result of source.values()) {
      const list = output.get(result.account_id) ?? [];
      list.push(result);
      output.set(result.account_id, list);
    }
  }
  return {
    modelResults,
    modelPrecheckResults,
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
