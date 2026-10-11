import { z } from "zod";

// Go validates Unicode code points, not UTF-16 code units. Passwords stay raw.
export function credentialLength(value: string): number {
  return Array.from(value).length;
}

export const consoleUsernameSchema = z
  .string()
  .trim()
  .refine((value) => credentialLength(value) >= 2, "账号至少 2 个字符")
  .refine((value) => credentialLength(value) <= 80, "账号不能超过 80 个字符");

export function consolePasswordSchema(label = "密码") {
  return z
    .string()
    .refine((value) => credentialLength(value) >= 10, `${label}至少 10 个字符`)
    .refine((value) => credentialLength(value) <= 256, `${label}不能超过 256 个字符`);
}
