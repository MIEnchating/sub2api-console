import { z } from "zod";
import type { KumaResource, KumaResourceKind } from "@/api";

const ids = z.array(z.number().int().positive());
const notificationSchema = z.object({
  name: z.string().max(150),
  type: z.string(),
  default: z.boolean(),
  active: z.boolean(),
  endpoint: z.string().max(8192),
  token: z.string().max(8192),
  username: z.string().max(8192),
  password: z.string().max(8192),
  endpoint_configured: z.boolean().optional(),
  token_configured: z.boolean().optional(),
  username_configured: z.boolean().optional(),
  password_configured: z.boolean().optional(),
  chat_id: z.string(),
  smtp_host: z.string(),
  smtp_port: z.number().int().min(1).max(65535),
  smtp_secure: z.boolean(),
  from: z.string(),
  to: z.string(),
  topic: z.string(),
});
const maintenanceSchema = z.object({
  title: z.string().max(150),
  description: z.string().max(10000),
  strategy: z.string(),
  active: z.boolean(),
  timezone: z.string(),
  start: z.string(),
  end: z.string(),
  cron: z.string().max(256),
  duration_minutes: z.number().int().min(1).max(525600),
  interval_days: z.number().int().min(1).max(365),
  start_time: z.string(),
  end_time: z.string(),
  weekdays: z.array(z.number().int().min(0).max(6)),
  days_of_month: z.array(z.number().int().min(1).max(31)),
  last_day: z.boolean(),
  monitor_ids: ids,
  status_page_ids: ids,
});
const statusPageSchema = z.object({
  title: z.string().max(150),
  slug: z.string(),
  description: z.string().max(10000),
  theme: z.string(),
  footer: z.string().max(10000),
  show_tags: z.boolean(),
  show_powered_by: z.boolean(),
  show_certificate_expiry: z.boolean(),
  domains: z.array(z.string()),
  groups: z.array(
    z.object({
      id: z.number().int().positive().optional(),
      name: z.string().min(1, "请输入展示分组名称").max(150),
      monitorList: z.array(
        z.object({
          id: z.number().int().positive(),
          sendUrl: z.boolean(),
          url: z.string().optional(),
        }),
      ),
    }),
  ),
});
export const resourceSchema = z
  .object({
    kind: z.enum(["notifications", "maintenance", "status-pages"]),
    notification: notificationSchema,
    maintenance: maintenanceSchema,
    status_page: statusPageSchema,
  })
  .superRefine((v, ctx) => {
    const add = (path: (string | number)[], message: string): void =>
      ctx.addIssue({ code: "custom", path, message });
    if (v.kind === "notifications") {
      const n = v.notification;
      if (!n.name.trim()) add(["notification", "name"], "请输入通知渠道名称");
      if (n.endpoint) {
        try {
          const u = new URL(n.endpoint);
          if (!["http:", "https:"].includes(u.protocol) || u.username || u.password)
            throw new Error();
        } catch {
          add(["notification", "endpoint"], "请输入有效的 HTTP(S) 服务地址");
        }
      }
      if (["webhook", "discord", "ntfy"].includes(n.type) && !n.endpoint && !n.endpoint_configured)
        add(["notification", "endpoint"], "请输入通知服务地址");
      if (n.type === "telegram") {
        if (!n.token && !n.token_configured) add(["notification", "token"], "请输入 Bot Token");
        if (!n.chat_id.trim()) add(["notification", "chat_id"], "请输入聊天 ID");
      }
      if (n.type === "ntfy" && !n.topic.trim()) add(["notification", "topic"], "请输入主题");
      if (n.type === "smtp") {
        if (!n.smtp_host.trim()) add(["notification", "smtp_host"], "请输入 SMTP 主机");
        for (const k of ["from", "to"] as const)
          if (!n[k].includes("@")) add(["notification", k], "请输入邮件地址");
      }
    }
    if (v.kind === "maintenance") {
      const m = v.maintenance;
      if (!m.title.trim()) add(["maintenance", "title"], "请输入维护标题");
      if (m.strategy === "single" && (!m.start || !m.end))
        add(["maintenance", "end"], "单次维护必须填写开始和结束时间");
      if (m.start && m.end && m.end <= m.start)
        add(["maintenance", "end"], "结束时间必须晚于开始时间");
      if (m.strategy === "cron" && !/^\S+(?:\s+\S+){4,5}$/.test(m.cron.trim()))
        add(["maintenance", "cron"], "请输入 5 或 6 段 Cron 表达式");
      if (m.strategy === "recurring-weekday" && !m.weekdays.length)
        add(["maintenance", "weekdays"], "请至少选择一天");
      if (m.strategy === "recurring-day-of-month" && !m.days_of_month.length && !m.last_day)
        add(["maintenance", "days_of_month"], "请至少填写一个日期");
    }
    if (v.kind === "status-pages") {
      const p = v.status_page;
      if (!p.title.trim()) add(["status_page", "title"], "请输入状态页标题");
      if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(p.slug) || p.slug.length > 100)
        add(["status_page", "slug"], "路径仅支持小写字母、数字与短横线");
      p.domains.forEach((d, i) => {
        if (!d || /[\s/?#@:]/.test(d))
          add(["status_page", "domains", i], "域名不能包含协议、端口或路径");
      });
    }
  });
export type ResourceValues = z.infer<typeof resourceSchema>;
export function resourceDefaults(kind: KumaResourceKind, item?: KumaResource): ResourceValues {
  return {
    kind,
    notification: {
      name: "",
      type: "webhook",
      default: false,
      active: true,
      endpoint: "",
      token: "",
      username: "",
      password: "",
      chat_id: "",
      smtp_host: "",
      smtp_port: 587,
      smtp_secure: false,
      from: "",
      to: "",
      topic: "",
      ...item?.notification,
    },
    maintenance: {
      title: "",
      description: "",
      strategy: "manual",
      active: true,
      timezone: "Asia/Shanghai",
      start: "",
      end: "",
      cron: "0 2 * * *",
      duration_minutes: 60,
      interval_days: 1,
      start_time: "02:00",
      end_time: "03:00",
      ...item?.maintenance,
      weekdays: item?.maintenance?.weekdays ?? [],
      days_of_month: item?.maintenance?.days_of_month ?? [],
      last_day: item?.maintenance?.last_day ?? false,
      monitor_ids: item?.maintenance?.monitor_ids ?? [],
      status_page_ids: item?.maintenance?.status_page_ids ?? [],
    },
    status_page: {
      title: "",
      slug: "",
      description: "",
      theme: "auto",
      footer: "",
      show_tags: false,
      show_powered_by: true,
      show_certificate_expiry: false,
      ...item?.status_page,
      domains: item?.status_page?.domains ?? [],
      groups: item?.status_page?.groups ?? [],
    },
  };
}
