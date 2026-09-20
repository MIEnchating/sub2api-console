import { z } from "zod";
export const loginInputSchema = z
  .object({
    kind: z.enum(["password", "email_code", "totp_code"]),
    value: z.string().min(1, "请输入验证内容").max(4096, "输入过长"),
  })
  .superRefine((value, context) => {
    if (value.kind !== "password" && !/^\d{6}$/.test(value.value)) {
      context.addIssue({ code: "custom", path: ["value"], message: "请输入六位数字验证码" });
    }
  });
export type LoginInputValues = z.infer<typeof loginInputSchema>;
