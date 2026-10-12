import { z } from "zod";

export const bindingCleanupSchema = z.object({
  enabled: z.boolean().optional(),
  max_per_round: z.number().int().min(1).max(10000).optional(),
});
