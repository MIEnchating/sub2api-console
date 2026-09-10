import { z } from "zod";
import { requestProfiles, parseRequestProfile } from "./request-profiles";
import type { KumaTemplate, KumaTemplateDetail } from "@/api";
import { httpMethods } from "../constants";
import { monitorOptionsSchema, defaultMonitorOptions } from "./schemas";

const templateMonitoringSchema = monitorOptionsSchema
  .pick({
    timeout: true,
    retry_interval: true,
    max_retries: true,
    max_redirects: true,
    accepted_status_codes: true,
    ignore_tls: true,
    upside_down: true,
    hostname: true,
    port: true,
    keyword: true,
    dns_record_type: true,
    dns_resolver: true,
  })
  .extend({
    type: z.enum(["http", "keyword", "port", "ping", "dns", "push", "group"]),
    url: z
      .string()
      .trim()
      .max(4096)
      .refine((value) => {
        if (!value) return true;
        try {
          const url = new URL(value);
          return (
            ["http:", "https:"].includes(url.protocol) &&
            !url.username &&
            !url.password &&
            !url.hash
          );
        } catch {
          return false;
        }
      }, "请输入有效的 HTTP(S) 监控地址"),
    interval: z.number().int().min(20, "检测间隔至少 20 秒").max(86400),
  })
  .superRefine((value, ctx) => {
    for (const code of value.accepted_status_codes) {
      const parts = code.split("-").map(Number);
      if (!/^[1-5]\d{2}(-[1-5]\d{2})?$/.test(code) || (parts.length === 2 && parts[0]! > parts[1]!))
        ctx.addIssue({
          code: "custom",
          path: ["accepted_status_codes"],
          message: "请输入有效状态码范围，例如 200-299",
        });
    }
    if (["port", "ping", "dns"].includes(value.type) && !value.hostname.trim())
      ctx.addIssue({ code: "custom", path: ["hostname"], message: "请输入主机名或 IP" });
    if (value.type === "port" && value.port < 1)
      ctx.addIssue({ code: "custom", path: ["port"], message: "请输入有效端口" });
    if (value.type === "keyword" && !value.keyword.trim())
      ctx.addIssue({ code: "custom", path: ["keyword"], message: "请输入匹配关键字" });
  });

export const templateSchema = z.object({
  monitoring: templateMonitoringSchema,
  body_encoding: z.enum(["json", "form", "xml"]),
  clear_url: z.boolean(),
  request_profile: z.enum(requestProfiles),
  model: z.string().trim().max(200),
  revision: z.number().int().nonnegative(),
  name: z.string().trim().min(1, "请输入模板名称").max(150, "名称最多 150 个字符"),
  method: z.enum(httpMethods),
  headers: z
    .string()
    .max(32768, "请求头过长")
    .refine((value) => {
      if (!value) return true;
      try {
        const parsed: unknown = JSON.parse(value);
        return (
          !!parsed &&
          typeof parsed === "object" &&
          !Array.isArray(parsed) &&
          Object.entries(parsed).every(
            ([key, item]) => typeof item === "string" && !/[\r\n]/.test(key + item),
          )
        );
      } catch {
        return false;
      }
    }, "请求头须为 JSON 对象，值为字符串且不含换行符"),
  body: z.string().max(65536, "请求体过长"),
  auth_method: z.enum(["none", "basic", "bearer"]),
  auth_username: z.string().max(4096),
  auth_password: z.string().max(4096),
  clear_headers: z.boolean(),
  clear_body: z.boolean(),
  clear_auth: z.boolean(),
});
export type TemplateValues = z.infer<typeof templateSchema>;
export function templateDefaults(
  item?: KumaTemplate & Partial<KumaTemplateDetail>,
): TemplateValues {
  return {
    monitoring: {
      type: "http",
      url: "",
      interval: 60,
      timeout: defaultMonitorOptions.timeout,
      retry_interval: defaultMonitorOptions.retry_interval,
      max_retries: 0,
      max_redirects: 10,
      accepted_status_codes: ["200-299"],
      ignore_tls: false,
      upside_down: false,
      hostname: "",
      port: 443,
      keyword: "",
      dns_record_type: "A",
      dns_resolver: "1.1.1.1",
      ...item?.monitoring,
    } as TemplateValues["monitoring"],
    body_encoding:
      item?.body_encoding === "form" || item?.body_encoding === "xml" ? item.body_encoding : "json",
    clear_url: false,
    request_profile: parseRequestProfile(item?.request_profile),
    model: item?.model ?? "claude-sonnet-4-6",
    revision: item?.revision ?? 0,
    name: item?.name ?? "",
    method: (item?.method ?? "POST") as TemplateValues["method"],
    headers: item?.headers ?? "",
    body: item?.body ?? "",
    auth_method: (item?.auth_method ?? "none") as TemplateValues["auth_method"],
    auth_username: "",
    auth_password: "",
    clear_headers: false,
    clear_body: false,
    clear_auth: false,
  };
}
