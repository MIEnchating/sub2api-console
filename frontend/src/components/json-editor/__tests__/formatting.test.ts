import { describe, expect, it } from "vitest";
import { formatJson, jsonErrors } from "../json-document";

describe("JSON 文档处理", () => {
  it("格式化时保留大整数、科学计数法、重复键和原始数值精度", () => {
    const input = '{"id":9007199254740993,"price":1.234567890123456789e-10,"id":2}';
    expect(formatJson(input)).toBe(
      '{\n  "id": 9007199254740993,\n  "price": 1.234567890123456789e-10,\n  "id": 2\n}',
    );
  });
  it("输入不合法时保留原文并返回语法错误位置", () => {
    expect(formatJson('{"a":}')).toBe('{"a":}');
    expect(jsonErrors('{"a":}')[0].offset).toBe(5);
  });
  it("留空时不补写空对象以保留凭据字段语义", () => {
    expect(formatJson("  ")).toBe("  ");
    expect(jsonErrors("")).toEqual([]);
  });
  it("输入注释或尾随逗号时按严格 JSON 拒绝", () => {
    expect(jsonErrors('{"a":1,}')).not.toHaveLength(0);
    expect(jsonErrors('{/* note */ "a":1}')).not.toHaveLength(0);
  });
});
