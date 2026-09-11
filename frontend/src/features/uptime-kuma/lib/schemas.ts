import { z } from "zod";

function validURL(value: string): boolean {
  try {
    const url = new URL(value);
    return (
      ["http:", "https:"].includes(url.protocol) && !url.username && !url.password && !url.hash
    );
  } catch {
    return false;
  }
}

export const configSchema = z.object({
  base_url: z
    .string()
    .trim()
    .max(2048)
    .refine(
      (value) => validURL(value) && !new URL(value).search,
      "请输入不含查询参数的 HTTP(S) 服务地址",
    ),
  api_key: z.string().trim().max(4096, "API 密钥过长"),
  username: z.string().trim().max(255, "账号过长"),
  password: z.string().max(4096, "密码过长"),
  otp: z
    .string()
    .trim()
    .refine((value) => value === "" || /^\d{6}$/.test(value), "请输入 6 位两步验证码"),
  disable_management: z.boolean(),
});
export type ConfigValues = z.infer<typeof configSchema>;

export const monitorOptionsSchema = z.object({
  hostname: z.string().trim().max(253),
  port: z.number().int().min(0).max(65535),
  keyword: z.string().max(4096),
  dns_record_type: z.string(),
  dns_resolver: z.string().trim(),
  method: z.string(),
  timeout: z.number().int().min(1).max(3600),
  retry_interval: z.number().int().min(20).max(86400),
  max_retries: z.number().int().min(0).max(100),
  resend_interval: z.number().int().min(0).max(10000),
  max_redirects: z.number().int().min(0).max(100),
  ignore_tls: z.boolean(),
  upside_down: z.boolean(),
  accepted_status_codes: z.array(z.string()).min(1),
  notification_ids: z.array(z.number().int().positive()),
  auth_method: z.string(),
  headers: z.string().max(32768),
  body: z.string().max(65536),
  auth_username: z.string().max(4096),
  auth_password: z.string().max(4096),
  clear_headers: z.boolean(),
  clear_body: z.boolean(),
  clear_auth: z.boolean(),
});
export const defaultMonitorOptions: z.infer<typeof monitorOptionsSchema> = {
  hostname: "",
  port: 443,
  keyword: "",
  dns_record_type: "A",
  dns_resolver: "1.1.1.1",
  method: "GET",
  timeout: 16,
  retry_interval: 60,
  max_retries: 0,
  resend_interval: 0,
  max_redirects: 10,
  ignore_tls: false,
  upside_down: false,
  accepted_status_codes: ["200-299"],
  notification_ids: [],
  auth_method: "none",
  headers: "",
  body: "",
  auth_username: "",
  auth_password: "",
  clear_headers: false,
  clear_body: false,
  clear_auth: false,
};

export const monitorSchema = z
  .object({
    name: z.string().trim().min(1, "请输入监控项名称").max(150, "名称不能超过 150 个字符"),
    template_model: z
      .string()
      .trim()
      .max(200, "模型名称最多为 200 个字符")
      .refine((value) => !/[\s\p{Cc}]/u.test(value), "模型名称不能包含空白或控制字符")
      .optional(),
    template_clear: z.boolean().optional(),
    template_retain: z.boolean().optional(),
    template_id: z.string().optional(),
    template_revision: z.number().int().nonnegative().optional(),
    template_auth_override: z.boolean().optional(),
    template_settings_override: z.boolean().optional(),
    type: z.string().min(1),
    options: monitorOptionsSchema.optional(),
    url: z
      .string()
      .trim()
      .max(4096)
      .refine((value) => value === "" || validURL(value), "请输入有效的 HTTP(S) 监控地址"),
    interval: z
      .number()
      .int("请输入整数秒数")
      .min(20, "检测间隔至少为 20 秒")
      .max(86400, "检测间隔最多为 86400 秒"),
    parent: z.number().int().positive().nullable(),
  })
  .superRefine((values, ctx) => {
    const o = values.options;
    if (!o) return;
    if (
      ["http", "keyword"].includes(values.type) &&
      values.template_id &&
      values.template_auth_override
    ) {
      if (o.auth_method === "basic" && !o.auth_username)
        ctx.addIssue({
          code: "custom",
          path: ["options", "auth_username"],
          message: "请输入鉴权用户名",
        });
      if (["basic", "bearer"].includes(o.auth_method) && !o.auth_password)
        ctx.addIssue({
          code: "custom",
          path: ["options", "auth_password"],
          message: "请输入鉴权密码或 Token",
        });
    }
    if (
      ["port", "ping", "dns"].includes(values.type) &&
      (!o.hostname || /[\s/\\?#@]/.test(o.hostname))
    )
      ctx.addIssue({
        code: "custom",
        path: ["options", "hostname"],
        message: "请输入有效的主机名或 IP 地址",
      });
    if (values.type === "port" && o.port < 1)
      ctx.addIssue({
        code: "custom",
        path: ["options", "port"],
        message: "TCP 端口必须为 1 到 65535",
      });
    if (values.type === "keyword" && !o.keyword.trim())
      ctx.addIssue({
        code: "custom",
        path: ["options", "keyword"],
        message: "请输入要匹配的关键字",
      });
    if (o.headers) {
      try {
        const h: unknown = JSON.parse(o.headers);
        if (
          !h ||
          Array.isArray(h) ||
          typeof h !== "object" ||
          Object.values(h).some((v) => typeof v !== "string")
        )
          throw new Error();
      } catch {
        ctx.addIssue({
          code: "custom",
          path: ["options", "headers"],
          message: "请求头须为 JSON 对象，值须为字符串",
        });
      }
    }
    for (const code of o.accepted_status_codes)
      if (!/^[1-5]\d{2}(-[1-5]\d{2})?$/.test(code))
        ctx.addIssue({
          code: "custom",
          path: ["options", "accepted_status_codes"],
          message: "正常状态码格式：200-299 或 301",
        });
  });
export type MonitorValues = z.infer<typeof monitorSchema>;
