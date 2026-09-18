import { expect, it } from "vitest";

import { billingExpressionsEqual } from "../billing-expression-comparison";

const expression = 'len <= 200000 ? tier("base", p * 3 + c * 15) : tier("long", p * 6 + c * 30)';

it("单档计费加法项换序且金额使用等价小数时判定一致", () => {
  expect(
    billingExpressionsEqual(
      'tier("base", p * 3 + c * 15 + cr * 0.3)',
      'tier("base", cr * 3e-1 + p * 3.0 + c * 15)',
    ),
  ).toBe(true);
});

it.each([
  ["条件边界改变", expression.replace("<= 200000", "< 200000")],
  ["条件数字变成字符串", expression.replace("200000", '"200000"')],
  ["档位名称含有不同空格", expression.replace('"base"', '"ba se"')],
  ["某一档单价改变", expression.replace("p * 6", "p * 7")],
  ["重复计费项", expression.replace("p * 3", "p * 3 + p * 3")],
  ["新增媒体计费项", expression.replace("p * 3", "p * 3 + ao * 1")],
  ["运算符改变", expression.replace("p * 3 + c * 15", "p * 3 - c * 15")],
  ["计费乘数改变", expression.replace("p * 3", "p * 3 * 2")],
  ["新增附加费", `${expression} + 100`],
])("%s时不将相同的可见单价视为表达式一致", (_label, changed) => {
  expect(billingExpressionsEqual(expression, changed)).toBe(false);
});

it("超出安全整数范围的不同条件阈值不被数值解析合并", () => {
  expect(
    billingExpressionsEqual(
      expression.replace("200000", "9007199254740992"),
      expression.replace("200000", "9007199254740993"),
    ),
  ).toBe(false);
});

it("表达式缺失时与已配置表达式不一致", () => {
  expect(billingExpressionsEqual("", expression)).toBe(false);
});

it("遇到解析器不支持的不同语法时保守返回不一致", () => {
  expect(billingExpressionsEqual("let rate = 3; p * rate", "let rate = 4; p * rate")).toBe(false);
});

it("遇到解析器不支持但原文相同的语法时仍判定一致", () => {
  expect(billingExpressionsEqual("let rate = 3; p * rate", "let rate = 3; p * rate")).toBe(true);
});
