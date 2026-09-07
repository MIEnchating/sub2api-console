import { describe, expect, it } from "vitest";
import type { NewAPIModelPrice, Sub2APIModelPrice } from "@/api";
import { modelPriceDifferenceRows, newAPIPriceComparisonStatus } from "../model-prices";

const configured: NewAPIModelPrice = {
  model: "minimax-m3",
  input_ratio: "0.3",
  completion_ratio: "4",
  cache_ratio: "0.2",
};
const remote: Sub2APIModelPrice = {
  model: "minimax-m3",
  source: "sub2api",
  input_price: "0.0000006",
  output_price: "0.0000024",
  model_ratio: "0.3",
  completion_ratio: "4",
  cache_ratio: "0.2",
  create_cache_ratio: "0",
  image_ratio: "0",
};

describe("Sub2API 默认零价比较", () => {
  it("可选默认零价与未配置不产生差异，并隐藏无有效价格的项目", () => {
    expect(newAPIPriceComparisonStatus(configured, [remote])).toBe("matched");
    const rows = modelPriceDifferenceRows(configured, remote);
    expect(
      rows.some((row) => row.label.startsWith("缓存写入") || row.label.startsWith("图片输入")),
    ).toBe(false);
    expect(rows.find((row) => row.label.startsWith("缓存读取"))).toMatchObject({
      configured: "0.12",
      remote: "0.12",
      matched: true,
    });
  });
  it("平台已有非零可选价格时仍显示与默认零价的差异", () => {
    const price = { ...configured, image_ratio: "2" };
    expect(newAPIPriceComparisonStatus(price, [remote])).toBe("mismatched");
    expect(
      modelPriceDifferenceRows(price, remote).find((row) => row.label.startsWith("图片输入")),
    ).toMatchObject({ configured: "1.2", remote: "0", matched: false });
  });
  it("公开远程价卡的明确零价与未配置仍有差异", () => {
    expect(newAPIPriceComparisonStatus(configured, [{ ...remote, source: "remote" }])).toBe(
      "mismatched",
    );
  });
  it("输出和缓存读取的明确零价不会被忽略", () => {
    expect(
      newAPIPriceComparisonStatus(configured, [
        { ...remote, completion_ratio: "0", cache_ratio: "0" },
      ]),
    ).toBe("mismatched");
    expect(
      modelPriceDifferenceRows(configured, { ...remote, completion_ratio: "0" }).find((row) =>
        row.label.startsWith("输出价格"),
      ),
    ).toMatchObject({ remote: "0", matched: false });
  });
});
