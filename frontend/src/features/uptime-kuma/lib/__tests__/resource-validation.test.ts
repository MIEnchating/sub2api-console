import { describe, expect, it } from "vitest";
import { resourceDefaults, resourceSchema } from "../resource-schemas";
import { defaultMonitorOptions, monitorSchema } from "../schemas";

describe("Uptime Kuma 管理参数", () => {
  it("新增 Webhook 缺少地址时阻止保存，已有地址留空保留", () => {
    const value = resourceDefaults("notifications");
    value.notification.name = "运维通知";
    expect(resourceSchema.safeParse(value).success).toBe(false);
    value.notification.endpoint_configured = true;
    expect(resourceSchema.safeParse(value).success).toBe(true);
  });
  it("单次维护结束早于开始时拒绝保存", () => {
    const value = resourceDefaults("maintenance");
    Object.assign(value.maintenance, {
      title: "升级",
      strategy: "single",
      start: "2026-10-01T02:00",
      end: "2026-10-01T01:00",
    });
    expect(resourceSchema.safeParse(value).success).toBe(false);
    value.maintenance.end = "2026-10-01T03:00";
    expect(resourceSchema.safeParse(value).success).toBe(true);
  });
  it("状态页路径含斜杠时拒绝保存，合法展示分组可提交", () => {
    const value = resourceDefaults("status-pages");
    Object.assign(value.status_page, { title: "服务状态", slug: "../admin" });
    expect(resourceSchema.safeParse(value).success).toBe(false);
    value.status_page.slug = "service-status";
    value.status_page.groups = [{ name: "API", monitorList: [{ id: 19, sendUrl: false }] }];
    expect(resourceSchema.safeParse(value).success).toBe(true);
  });
  it("TCP 端口越界或主机含 URL 协议时拒绝创建", () => {
    const input = {
      name: "API",
      type: "port",
      url: "",
      interval: 60,
      parent: null,
      options: { ...defaultMonitorOptions, hostname: "https://api.example", port: 70000 },
    };
    expect(monitorSchema.safeParse(input).success).toBe(false);
    input.options.hostname = "api.example";
    input.options.port = 443;
    expect(monitorSchema.safeParse(input).success).toBe(true);
  });
  it("请求头不是键值对象时拒绝保存", () => {
    const input = {
      name: "API",
      type: "http",
      url: "https://api.example",
      interval: 60,
      parent: null,
      options: { ...defaultMonitorOptions, headers: "[]" },
    };
    expect(monitorSchema.safeParse(input).success).toBe(false);
    input.options.headers = '{"Content-Type":"application/json"}';
    expect(monitorSchema.safeParse(input).success).toBe(true);
  });
});
