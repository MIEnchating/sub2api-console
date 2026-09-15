import { describe, expect, it } from "vitest";
import { configSchema, defaultMonitorOptions, monitorSchema } from "../schemas";

describe("Uptime Kuma 参数校验", () => {
  it("正常状态码区间倒序时在状态码字段拒绝提交", () => {
    const result = monitorSchema.safeParse({
      name: "接口",
      type: "http",
      url: "https://monitor.example",
      interval: 60,
      parent: null,
      options: { ...defaultMonitorOptions, accepted_status_codes: ["299-200"] },
    });
    expect(result.success).toBe(false);
    if (!result.success)
      expect(result.error.issues[0].path).toEqual(["options", "accepted_status_codes"]);
  });

  it("正常状态码包含递增区间和单项时允许提交", () => {
    expect(
      monitorSchema.safeParse({
        name: "接口",
        type: "http",
        url: "https://monitor.example",
        interval: 60,
        parent: null,
        options: { ...defaultMonitorOptions, accepted_status_codes: ["200-299", "301"] },
      }).success,
    ).toBe(true);
  });

  it.each([
    "javascript:alert(1)",
    "https://user:secret@kuma.example",
    "https://kuma.example?key=secret",
  ])("服务地址为 %s 时拒绝提交", (base_url) => {
    expect(
      configSchema.safeParse({
        base_url,
        api_key: "key",
        username: "",
        password: "",
        otp: "",
        disable_management: false,
      }).success,
    ).toBe(false);
  });
  it("检测间隔少于 20 秒时拒绝提交", () => {
    expect(
      monitorSchema.safeParse({
        name: "测试",
        type: "http",
        url: "https://monitor.example",
        interval: 19,
        parent: null,
      }).success,
    ).toBe(false);
  });
});
