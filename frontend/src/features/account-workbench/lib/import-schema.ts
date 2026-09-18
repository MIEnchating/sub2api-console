import { z } from "zod";
export const importSchema = z
  .object({
    content: z
      .string()
      .trim()
      .min(1, "请粘贴账号资料或上传文件")
      .max(4 * 1024 * 1024, "输入不能超过 4 MB"),
    action: z.enum(["import", "export"]),
    template_id: z.string(),
    check: z.boolean(),
    promote: z.boolean(),
    model: z.string().trim().min(1, "请输入检测模型").max(200),
    proxy_enabled: z.boolean(),
    proxy_url: z.string(),
  })
  .refine((value) => !value.proxy_enabled || value.proxy_url.trim() !== "", {
    path: ["proxy_url"],
    message: "启用代理后请填写代理地址",
  });
export type ImportValues = z.infer<typeof importSchema>;
export const importDefaults: ImportValues = {
  content: "",
  action: "import",
  template_id: "",
  check: true,
  promote: true,
  model: "gpt-5.6-sol",
  proxy_enabled: false,
  proxy_url: "",
};
