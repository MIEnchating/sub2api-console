import { describe, expect, it } from "vitest";
import type { DictionaryEntry } from "@/api";
import { orderByDictionary } from "../dictionary-order";

describe("消费字典排序", () => {
  it("字典逆序时保留选项的稳定 ID、大小写和附加属性，未登记值置后", () => {
    const items = [
      { id: "A", label: "相同名称", count: 1 },
      { id: "B", label: "相同名称", count: 2 },
      { id: "C", label: "新项", count: 3 },
    ];
    const entries = [
      { value: "B", enabled: true },
      { value: "A", enabled: true },
    ] as DictionaryEntry[];
    expect(orderByDictionary(items, entries, (item) => item.id)).toEqual([
      items[1],
      items[0],
      items[2],
    ]);
    expect(items[0].id).toBe("A");
  });
  it("字典未就绪时保留现有选项", () => {
    expect(orderByDictionary(["C", "A"], undefined, (value) => value)).toEqual(["C", "A"]);
  });
});

it("仅有名称的展示项遇到同名字典条目时使用首次出现顺序，不做模糊匹配", () => {
  const entries = [
    { value: "2", name: "B", enabled: true },
    { value: "1", name: "A", enabled: true },
    { value: "3", name: "B", enabled: true },
  ] as DictionaryEntry[];
  expect(orderByDictionary(["A", "B", "b"], entries, (value) => value, "name")).toEqual([
    "B",
    "A",
    "b",
  ]);
});

it("字典没有可用条目时保留空列表和原始选项", () => {
  const entries = [{ value: "B", enabled: false }] as DictionaryEntry[];
  expect(orderByDictionary([], entries, String)).toEqual([]);
  expect(orderByDictionary(["A", "B"], entries, String)).toEqual(["A", "B"]);
});
