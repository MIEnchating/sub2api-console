import { z } from "zod";

export const platformProbeSchema = z.object({
  platform: z.string().trim().min(1, "请选择平台").max(64, "平台标识不能超过 64 个字符"),
  model: z.string().trim().min(1, "请输入探活模型").max(256, "模型名称不能超过 256 个字符"),
});

export type PlatformProbeForm = z.infer<typeof platformProbeSchema>;
