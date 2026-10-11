import { z } from "zod";
import { consoleUsernameSchema, credentialLength } from "@/features/auth/lib/credential-schema";

export const profileSchema = z
  .object({
    username: consoleUsernameSchema,
    current_password: z.string().min(1, "请输入当前密码"),
    new_password: z
      .string()
      .refine((value) => credentialLength(value) <= 256, "新密码不能超过 256 个字符"),
    confirm_password: z
      .string()
      .refine((value) => credentialLength(value) <= 256, "确认密码不能超过 256 个字符"),
  })
  .superRefine((value, context) => {
    if (!value.new_password && !value.confirm_password) return;
    if (credentialLength(value.new_password) < 10) {
      context.addIssue({ code: "custom", path: ["new_password"], message: "新密码至少 10 个字符" });
    }
    if (value.new_password === value.current_password) {
      context.addIssue({
        code: "custom",
        path: ["new_password"],
        message: "新密码不能与当前密码相同",
      });
    }
    if (value.new_password !== value.confirm_password) {
      context.addIssue({
        code: "custom",
        path: ["confirm_password"],
        message: "两次输入的新密码不一致",
      });
    }
  });

export type ProfileForm = z.infer<typeof profileSchema>;
