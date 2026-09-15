import { describe, expect, it } from "vitest";
import { mixedRunDefaults, mixedRunSchema } from "../lib/mixed-run-schema";

describe("账号导入输入", () => {
  it("混合原文保持数字精度、行序及JSON格式交给后端解析", () => {
    const content =
      '{ "account_id":9007199254740993 }\nrt_fixture\noperator@example.test----password';
    expect(mixedRunSchema.parse({ ...mixedRunDefaults, content }).content).toBe(content);
  });
  it("空输入和超过2MB内容阻止发送", () => {
    expect(mixedRunSchema.safeParse(mixedRunDefaults).success).toBe(false);
    expect(
      mixedRunSchema.safeParse({ ...mixedRunDefaults, content: "x".repeat(2 * 1024 * 1024 + 1) })
        .success,
    ).toBe(false);
  });
  it("导入模型过长时阻止发送，私有转换忽略已隐藏模型", () => {
    const value = {
      ...mixedRunDefaults,
      content: "operator@example.test",
      model: "m".repeat(201),
    };
    expect(mixedRunSchema.safeParse(value).success).toBe(false);
    expect(mixedRunSchema.safeParse({ ...value, export_only: true }).success).toBe(true);
  });
});
