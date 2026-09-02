import { z } from "zod";

export const newAPIPlatformSchema = z.object({
  name: z.string().trim().min(1, "请输入平台名称").max(120),
  base_url: z.string().trim().url("请输入有效的平台地址").max(2048),
  admin_key: z.string().trim().max(4096),
  user_id: z.string().trim().min(1, "请输入 User ID").max(128),
});

export type NewAPIPlatformValues = z.infer<typeof newAPIPlatformSchema>;

export const newAPIChannelSchema = z.object({
  name: z.string().trim().min(1, "请输入渠道名称").max(120),
  sub2api_group_id: z.string().trim().min(1, "请选择 Sub2API 分组"),
  base_url: z.string().trim().url("请输入有效的 Sub2API 服务地址").max(2048),
  service_key: z.string().trim().min(1, "请输入 Sub2API 服务密钥").max(4096),
  models: z.string().trim().min(1, "请输入至少一个模型"),
});

export type NewAPIChannelValues = z.infer<typeof newAPIChannelSchema>;

export function parseModelList(value: string): string[] {
  return Array.from(
    new Set(
      value
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  ).sort();
}
