import { z } from "zod";

export const newAPIPlatformSchema = z.object({
  name: z.string().trim().min(1, "请输入平台名称").max(120),
  base_url: z.string().trim().url("请输入有效的平台地址").max(2048),
  admin_key: z.string().trim().max(4096),
  user_id: z.string().trim().min(1, "请输入 User ID").max(128),
});

export type NewAPIPlatformValues = z.infer<typeof newAPIPlatformSchema>;

export const newAPIChannelSchema = z.object({
  sub2api_group_id: z.string().trim().min(1, "请选择 Sub2API 分组"),
  key_id: z.string().trim().min(1, "请先创建 Sub2API 密钥").max(128),
  models: z.array(z.string()).min(1, "请从上游获取并选择至少一个模型").max(500),
  newapi_groups: z.array(z.string()).min(1, "请选择至少一个 New API 分组").max(500),
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
