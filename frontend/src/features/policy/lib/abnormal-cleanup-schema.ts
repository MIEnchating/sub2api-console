import { z } from "zod";

export const abnormalCleanupSchema = z.object({
  enabled: z.boolean().optional(),
  action: z.enum(["pause", "disable", "delete"]).optional(),
  duration_minutes: z.number().int().min(1).max(525600).optional(),
  max_per_round: z.number().int().min(1).max(10000).optional(),
  keep_last_in_group: z.boolean().optional(),
});
