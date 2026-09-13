import { expect, it } from "vitest";
import { account } from "@/features/accounts/__tests__/fixtures";
import { defaultAnimationFilters, filterAnimationAccounts } from "../animation-filters";

const accounts = [
  {
    ...account,
    id: "1",
    name: "主账号",
    platform: "openai",
    groups: ["主组"],
    manual_priority: 0,
    upstream_host: "Primary.Example.Test",
  },
  {
    ...account,
    id: "2",
    name: "备用账号",
    platform: "anthropic",
    groups: ["主组", "备用组"],
    manual_priority: null,
  },
  {
    ...account,
    id: "3",
    name: "手动账号",
    platform: "openai",
    groups: ["备用组"],
    manual_priority: 3,
  },
];

it("组合筛选使用交集，人工优先值为零的账号也能匹配", () => {
  expect(
    filterAnimationAccounts(accounts, {
      query: "",
      group: "主组",
      platform: "openai",
      priority: "manual",
    }).map((item) => item.id),
  ).toEqual(["1"]);
});

it("自动调度筛选排除所有人工优先账号", () => {
  expect(
    filterAnimationAccounts(accounts, { ...defaultAnimationFilters, priority: "automatic" }).map(
      (item) => item.id,
    ),
  ).toEqual(["2"]);
});

it("关键词忽略首尾空格与 Host 大小写", () => {
  expect(
    filterAnimationAccounts(accounts, {
      ...defaultAnimationFilters,
      query: " primary.example.test ",
    }).map((item) => item.id),
  ).toEqual(["1"]);
});

it("无匹配或空账号列表时返回空结果，重置筛选后返回全部账号", () => {
  expect(
    filterAnimationAccounts(accounts, { ...defaultAnimationFilters, group: "不存在" }),
  ).toEqual([]);
  expect(filterAnimationAccounts([], defaultAnimationFilters)).toEqual([]);
  expect(filterAnimationAccounts(accounts, defaultAnimationFilters)).toEqual(accounts);
});
