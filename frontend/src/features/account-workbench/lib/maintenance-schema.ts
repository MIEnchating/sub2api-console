import { z } from "zod";
import type { WorkbenchMaintenance } from "../types";

export const maintenanceSchema = z.object({
  enabled: z.boolean(),
  check_enabled: z.boolean(),
  interval_minutes: z.number().int().min(1, "最少 1 分钟").max(1440, "最多 1440 分钟"),
  cooldown_minutes: z.number().int().min(0, "不能小于 0 分钟").max(1440, "最多 1440 分钟"),
  check_after_repair: z.boolean(),
  group_ids: z.array(z.string().regex(/^[1-9]\d*$/, "分组 ID 无效")).max(500),
});
export type MaintenanceValues = z.infer<typeof maintenanceSchema>;

export function maintenanceFormValues(value: WorkbenchMaintenance): MaintenanceValues {
  return {
    enabled: value.enabled,
    check_enabled: value.check_enabled ?? false,
    interval_minutes: value.interval_minutes,
    cooldown_minutes: value.cooldown_minutes,
    check_after_repair: value.check_after_repair,
    group_ids: value.group_ids,
  };
}
