import { z } from "zod";

const positiveInteger = z
  .string()
  .trim()
  .refine(
    (value) => /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= 10_000_000,
    "请输入 1 到 10000000 之间的整数",
  );

export function parseAccountModels(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  ].sort();
}

export function parseRetryStatusCodes(value: string): number[] {
  return [
    ...new Set(
      value
        .split(/[\s,，]+/)
        .map((item) => item.trim())
        .filter(Boolean)
        .map(Number),
    ),
  ].sort((left, right) => left - right);
}

export const accountCreationSettingsSchema = z.object({
  models: z
    .string()
    .refine((value) => parseAccountModels(value).length <= 100, "最多配置 100 个模型")
    .refine(
      (value) => parseAccountModels(value).every((model) => [...model].length <= 256),
      "模型名称不能超过 256 个字符",
    ),
  concurrency: positiveInteger,
  loadFactor: z
    .string()
    .trim()
    .refine(
      (value) =>
        value === "" ||
        (/^(?:\d+)(?:\.\d+)?$/.test(value) && Number.isFinite(Number(value)) && Number(value) >= 1),
      "请输入大于或等于 1 的十进制数",
    ),
  priority: positiveInteger,
  poolMode: z.boolean(),
  retryCount: z
    .string()
    .trim()
    .refine((value) => /^\d+$/.test(value) && Number(value) >= 0 && Number(value) <= 10, {
      message: "请输入 0 到 10 之间的整数",
    }),
  retryStatusCodes: z.string().refine((value) => {
    const tokens = value
      .split(/[\s,，]+/)
      .map((item) => item.trim())
      .filter(Boolean);
    return (
      tokens.length <= 32 &&
      tokens.every((token) => /^\d{3}$/.test(token) && Number(token) >= 100 && Number(token) <= 599)
    );
  }, "请输入最多 32 个 100 到 599 之间的状态码"),
});

export type AccountCreationSettingsValues = z.infer<typeof accountCreationSettingsSchema>;
