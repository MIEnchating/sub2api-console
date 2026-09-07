import { z } from "zod";

function parsePayload(value: string): unknown {
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return null;
  }
}

export const modelCheckConfigurationSchema = z.object({
  note: z.string().trim().max(200, "版本说明不能超过 200 个字符"),
  payload_json: z
    .string()
    .trim()
    .min(1, "请输入题库和画像配置")
    .refine((value) => {
      const parsed = parsePayload(value);
      return parsed !== null && typeof parsed === "object" && !Array.isArray(parsed);
    }, "配置必须是有效的 JSON 对象"),
});

export type ModelCheckConfigurationForm = z.infer<typeof modelCheckConfigurationSchema>;
