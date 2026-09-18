import { z } from "zod";

export const maintenanceSchema = z.object({
  enabled: z.boolean(),
  interval_minutes: z.number().int().min(1, "最少 1 分钟").max(1440, "最多 1440 分钟"),
  cooldown_minutes: z.number().int().min(0, "不能小于 0 分钟").max(1440, "最多 1440 分钟"),
  check_after_repair: z.boolean(),
  group_ids: z.array(z.string().regex(/^[1-9]\d*$/, "分组 ID 无效")).max(500),
});
export type MaintenanceValues = z.infer<typeof maintenanceSchema>;
