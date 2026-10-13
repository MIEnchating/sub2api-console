import { z } from "zod";
export const exclusionSchema = z.object({ stopScheduling: z.boolean() });
export type ExclusionForm = z.infer<typeof exclusionSchema>;
