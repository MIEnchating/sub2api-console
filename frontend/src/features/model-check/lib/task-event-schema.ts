import { z } from "zod";
import type { Task } from "@/api";
import { compareTimestamps, fractionalSeconds } from "@/lib/time-order";

const timestampSchema = z.iso
  .datetime({ offset: true })
  .refine((value) => fractionalSeconds(value).length <= 9);

export const taskEventSchema = z.object({
  id: z.string().min(1),
  skill: z.string(),
  operation: z.string(),
  status: z.enum([
    "queued",
    "running",
    "waiting_input",
    "succeeded",
    "partial",
    "failed",
    "cancelled",
  ]),
  progress: z.number(),
  message: z.string(),
  result: z.record(z.string(), z.unknown()),
  created_at: timestampSchema,
  updated_at: timestampSchema,
}) satisfies z.ZodType<Task>;

export function taskEventIsOlder(current: Task | undefined, next: Task): boolean {
  if (!current) return false;
  return compareTimestamps(next.updated_at, current.updated_at) < 0;
}
