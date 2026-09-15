import { z } from "zod";

export function historyEmails(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[\s,，;；]+/u)
        .filter(Boolean)
        .map((email) => email.toLowerCase()),
    ),
  ];
}

export const historyQuerySchema = z.object({
  search: z.string().trim().max(1024, "查询关键词不能超过 1024 个字符"),
  emails: z
    .string()
    .max(160500, "邮箱列表过长")
    .refine((value) => {
      const emails = historyEmails(value);
      return emails.length <= 500 && emails.every((email) => z.email().safeParse(email).success);
    }, "请填写最多 500 个完整邮箱，每行一个或用逗号分隔"),
});
export type HistoryQueryForm = z.infer<typeof historyQuerySchema>;
