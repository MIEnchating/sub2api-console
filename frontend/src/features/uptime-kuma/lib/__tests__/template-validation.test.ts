import { describe, expect, it } from "vitest";
import { templateDefaults, templateSchema } from "../template-schema";
describe("请求模板校验", () => {
  it("输入合法 JSON 请求头和任意请求体时允许保存", () => {
    expect(
      templateSchema.safeParse({
        ...templateDefaults(),
        name: "探测",
        headers: '{"Content-Type":"application/json"}',
        body: '{"messages":[]}',
      }).success,
    ).toBe(true);
  });
  it.each(["[]", '{"X-Key":1}', '{"X-Key":"line\\nvalue"}'])(
    "请求头为 %s 时拒绝保存",
    (headers) => {
      expect(
        templateSchema.safeParse({ ...templateDefaults(), name: "探测", headers }).success,
      ).toBe(false);
    },
  );
  it("模板名称为空时提供字段错误", () => {
    expect(templateSchema.safeParse(templateDefaults()).success).toBe(false);
  });
  it("检测参数超出范围或状态码倒序时阻止保存", () => {
    const value = { ...templateDefaults(), name: "检测" };
    value.monitoring.timeout = 0;
    value.monitoring.accepted_status_codes = ["299-200"];
    const result = templateSchema.safeParse(value);
    expect(result.success).toBe(false);
    if (!result.success)
      expect(result.error.issues.map((issue) => issue.path.join("."))).toEqual(
        expect.arrayContaining(["monitoring.timeout", "monitoring.accepted_status_codes"]),
      );
  });
  it("TCP 模板缺少主机和端口时阻止保存", () => {
    const value = { ...templateDefaults(), name: "TCP" };
    value.monitoring.type = "port";
    value.monitoring.port = 0;
    expect(templateSchema.safeParse(value).success).toBe(false);
  });
});
