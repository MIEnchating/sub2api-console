import { toast } from "sonner";

import type { Task } from "@/api";

export type ProbeTaskFeedback = {
  tone: "success" | "warning" | "error" | "info";
  title: string;
  description?: string;
};

function resultRows(task: Task): Record<string, unknown>[] {
  const raw = task.result.results;
  if (!Array.isArray(raw)) return [];
  return raw.filter(
    (row): row is Record<string, unknown> =>
      typeof row === "object" && row !== null && !Array.isArray(row),
  );
}

function nonnegativeNumber(value: unknown): number | null {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) return null;
  return value;
}

function count(value: unknown, fallback: number): number {
  const parsed = nonnegativeNumber(value);
  return parsed === null ? fallback : Math.trunc(parsed);
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

export function formatProbeDuration(durationMS: number | null): string | null {
  if (durationMS === null || !Number.isFinite(durationMS) || durationMS < 0) return null;
  if (durationMS < 1000) return `${Math.max(1, Math.round(durationMS))} 毫秒`;
  const seconds = Math.round((durationMS / 1000) * 100) / 100;
  return `${seconds} 秒`;
}

function taskDurationMS(task: Task): number | null {
  const createdAt = Date.parse(task.created_at);
  const updatedAt = Date.parse(task.updated_at);
  if (!Number.isFinite(createdAt) || !Number.isFinite(updatedAt) || updatedAt < createdAt) {
    return null;
  }
  return updatedAt - createdAt;
}

function description(parts: Array<string | null | undefined>): string | undefined {
  const visible = parts.filter((part): part is string => Boolean(part));
  return visible.length > 0 ? visible.join(" · ") : undefined;
}

export function probeTaskFeedback(task: Task, accountName?: string): ProbeTaskFeedback {
  const prefix = accountName ? `${accountName}：` : "";
  if (task.status === "cancelled") {
    return { tone: "info", title: `${prefix}探活已取消`, description: task.message || undefined };
  }

  const rows = resultRows(task);
  if (rows.length === 1) {
    const row = rows[0];
    const result = text(row.result);
    const duration = formatProbeDuration(
      nonnegativeNumber(row.duration_ms) ??
        nonnegativeNumber(task.result.duration_ms) ??
        taskDurationMS(task),
    );
    const failureReason = text(row.failure_reason);
    if (result === "通过") {
      return {
        tone: "success",
        title: `${prefix}探活通过`,
        description: description([duration ? `耗时 ${duration}` : null]),
      };
    }
    if (result === "跳过") {
      return {
        tone: "info",
        title: `${prefix}探活已跳过`,
        description: description([duration ? `耗时 ${duration}` : null, failureReason]),
      };
    }
    return {
      tone: "error",
      title: `${prefix}探活失败`,
      description: description([duration ? `耗时 ${duration}` : null, failureReason]),
    };
  }

  const passed = count(
    task.result.passed,
    rows.filter((row) => text(row.result) === "通过").length,
  );
  const skipped = count(
    task.result.skipped,
    rows.filter((row) => text(row.result) === "跳过").length,
  );
  const failed = count(task.result.failed, Math.max(0, rows.length - passed - skipped));
  const duration = formatProbeDuration(
    nonnegativeNumber(task.result.duration_ms) ?? taskDurationMS(task),
  );
  const summary = `通过 ${passed}，失败 ${failed}，跳过 ${skipped}`;
  if (failed === 0 && passed > 0) {
    return {
      tone: "success",
      title: `${prefix}探活通过`,
      description: description([summary, duration ? `耗时 ${duration}` : null]),
    };
  }
  if (failed > 0 && passed > 0) {
    return {
      tone: "warning",
      title: `${prefix}探活部分通过`,
      description: description([summary, duration ? `耗时 ${duration}` : null]),
    };
  }
  return {
    tone: "error",
    title: `${prefix}探活失败`,
    description: description([summary, duration ? `耗时 ${duration}` : null, task.message]),
  };
}

export function notifyProbeTaskResult(task: Task, accountName?: string): void {
  const feedback = probeTaskFeedback(task, accountName);
  const options = feedback.description ? { description: feedback.description } : undefined;
  if (feedback.tone === "success") toast.success(feedback.title, options);
  else if (feedback.tone === "warning") toast.warning(feedback.title, options);
  else if (feedback.tone === "info") toast.info(feedback.title, options);
  else toast.error(feedback.title, options);
}
