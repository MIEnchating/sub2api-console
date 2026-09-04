import { describe, expect, it } from "vitest";

import { formatModelPriceNumber, modelPriceNumbersEqual } from "../pricing-number";

describe("模型价格数字格式", () => {
  it("四舍五入浮点换算尾差并移除无意义的零", () => {
    expect(formatModelPriceNumber("4.399999999998")).toBe("4.4");
    expect(formatModelPriceNumber("1.000000000004")).toBe("1");
    expect(formatModelPriceNumber("0.279999999999997")).toBe("0.28");
  });

  it("保留八位以内的有效价格精度", () => {
    expect(formatModelPriceNumber("0.12345678")).toBe("0.12345678");
    expect(formatModelPriceNumber("0.123456789")).toBe("0.12345679");
  });

  it("比较时忽略尾差但不混淆真实差价和缺失值", () => {
    expect(modelPriceNumbersEqual("4.4", "4.399999999998")).toBe(true);
    expect(modelPriceNumbersEqual("0.26", "0.279999999999997")).toBe(false);
    expect(modelPriceNumbersEqual(undefined, "1.000000000004")).toBe(false);
  });
});
