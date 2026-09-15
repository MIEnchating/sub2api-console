import { describe, expect, it } from "vitest";
import { importSchema, maintenanceSchema, templateSchema } from "../lib/schemas";
import { maxInputBytes } from "../constants";

describe("账号工作台输入校验", () => {
  it("拒绝超过 2 MB 的导入内容", () => {
    const result = importSchema.safeParse({
      content: "x".repeat(maxInputBytes + 1),
      template_id: "",
      check_after_import: false,
      model: "",
    });
    expect(result.success).toBe(false);
  });

  it("启用导入后检测时要求模型名称", () => {
    const result = importSchema.safeParse({
      content: "rt_example",
      template_id: "",
      check_after_import: true,
      model: "",
    });
    expect(result.success).toBe(false);
    if (!result.success)
      expect(result.error.issues.some((issue) => issue.path.join(".") === "model")).toBe(true);
  });

  it("拒绝不合法邮箱域名并接受默认模板配置", () => {
    const result = templateSchema.safeParse({
      preferred: false,
      name: "默认 OAuth",
      priority: 0,
      match: { plan_type: "", email_domain: "not a domain" },
      config: {
        concurrency: 10,
        priority: 0,
        rate_multiplier: "1",
        group_ids: [],
        auto_pause_on_expired: true,
      },
    });
    expect(result.success).toBe(false);
  });

  it("自动维护的间隔必须为正整数", () => {
    const result = maintenanceSchema.safeParse({
      enabled: false,
      interval_minutes: 0,
      cooldown_minutes: 10,
      group_ids: [],
      check_after_repair: false,
      model: "",
    });
    expect(result.success).toBe(false);
  });
});
