import { z } from "zod";
import type { AutoInspectionStatus } from "@/api";

const summarySchema = z
  .object({
    channels: z.number(),
    probed: z.number(),
    samples: z.number(),
    fused: z.number(),
    recovered: z.number(),
    auth_recovered: z.number().optional(),
    applied: z.number(),
    cleaned_up: z.number(),
    alerts: z.number(),
  })
  .passthrough();
const completedStateSchema = z.enum(["succeeded", "partial", "failed", "cancelled"]);

export const autoInspectionStatusSchema = z
  .object({
    enabled: z.boolean(),
    interval_seconds: z.number(),
    account_rate_sync_interval_seconds: z.number().optional(),
    account_rate_sync_batch_size: z.number().optional(),
    account_rate_sync_batch_percent: z.number().optional(),
    running: z.boolean(),
    monitoring_configured: z.boolean(),
    monitoring_enabled: z.boolean(),
    monitoring_checked_at: z.string().nullable(),
    last_run_duration_ms: z.number(),
    last_summary: summarySchema,
    last_run_at: z.string().nullable(),
    next_run_at: z.string().nullable(),
    last_status: completedStateSchema.nullable(),
    last_error: z.string().nullable(),
    last_task_id: z.string().nullable(),
    queue: z
      .array(
        z
          .object({
            task_type: z.literal("inspection"),
            label: z.string(),
            state: z.enum(["ready", "waiting", "disabled", "blocked"]),
            scheduled_for: z.string().nullable(),
            detail: z.string(),
            target_count: z.number().nullable(),
            operations: z
              .array(
                z
                  .object({
                    operation: z.string(),
                    label: z.string(),
                    target_count: z.number().nullable(),
                    cycle: z.string(),
                    due: z.boolean(),
                  })
                  .passthrough(),
              )
              .nullable()
              .transform((value) => value ?? []),
          })
          .passthrough(),
      )
      .nullable()
      .transform((value) => value ?? []),
    heartbeat_history: z
      .array(
        z
          .object({
            checked_at: z.string(),
            completed_at: z.string().nullable(),
            status: z.enum(["running", ...completedStateSchema.options]),
            operations: z
              .array(z.string())
              .nullable()
              .transform((value) => value ?? []),
            operation_timings: z
              .array(
                z
                  .object({
                    operation: z.string(),
                    duration_seconds: z.number(),
                    started_at: z.string().nullish(),
                  })
                  .passthrough(),
              )
              .nullable()
              .transform((value) => value ?? []),
            task_id: z.string().nullable(),
            error: z.string().nullable(),
            skipped: z.boolean(),
            summary: summarySchema.nullish(),
            monitoring_enabled: z.boolean().nullish(),
          })
          .passthrough(),
      )
      .nullable()
      .transform((value) => value ?? []),
  })
  .passthrough() satisfies z.ZodType<AutoInspectionStatus>;
