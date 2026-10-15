import { z } from "zod";

export const vaultSchema = z.object({
  entry: z
    .string()
    .trim()
    .min(1, "请输入凭据名称")
    .refine((value) => Array.from(value).length <= 255, "凭据名称不能超过 255 个字符"),
  username: z.string(),
  password: z.string(),
  hosts: z.string(),
  headers: z.string(),
});

export type VaultForm = z.infer<typeof vaultSchema>;
export type MutableVaultField = "username" | "password" | "hosts" | "headers";
export const emptyVaultForm: VaultForm = {
  entry: "",
  username: "",
  password: "",
  hosts: "",
  headers: "",
};
