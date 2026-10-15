import { z } from "zod";

export const systemLogSearchSchema = z.object({
  requestId: z.string().trim(),
  timeRange: z.enum(["5m", "30m", "1h", "6h", "24h", "7d", "30d"]),
  level: z.enum(["", "debug", "info", "warn", "error"]),
});

export type SystemLogFilters = z.infer<typeof systemLogSearchSchema>;

export const defaultSystemLogFilters: SystemLogFilters = {
  requestId: "",
  timeRange: "1h",
  level: "",
};
