import { z } from "zod";

export function parseModelBlockPatterns(value: string): string[] {
  const patterns = new Map<string, string>();
  for (const line of value.split(/\r?\n/)) {
    const pattern = line.trim();
    if (pattern) patterns.set(pattern.toLocaleLowerCase(), pattern);
  }
  return Array.from(patterns.values()).sort((left, right) => left.localeCompare(right));
}

export const modelSyncSettingsSchema = z.object({
  blockedPatterns: z.string().superRefine((value, context) => {
    const patterns = parseModelBlockPatterns(value);
    if (patterns.length > 200) {
      context.addIssue({ code: z.ZodIssueCode.custom, message: "最多配置 200 条屏蔽规则" });
    }
    if (patterns.some((pattern) => Array.from(pattern).length > 256)) {
      context.addIssue({ code: z.ZodIssueCode.custom, message: "每条规则不能超过 256 个字符" });
    }
  }),
});

export type ModelSyncSettingsValues = z.infer<typeof modelSyncSettingsSchema>;
