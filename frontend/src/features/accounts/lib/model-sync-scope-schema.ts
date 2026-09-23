import { z } from "zod";

export const modelSyncScopeSchema = z.object({
  groups: z.array(z.string()).min(1, "请选择需要同步的分组"),
});
export type ModelSyncScopeValues = z.infer<typeof modelSyncScopeSchema>;
