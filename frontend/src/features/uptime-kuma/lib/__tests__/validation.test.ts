import { describe, expect, it } from "vitest";
import { configSchema, monitorSchema } from "../schemas";

describe("Uptime Kuma 参数校验", () => {
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
