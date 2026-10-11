import { expect, it } from "vitest";
import { createAccountSearchIndex, filterAccountSearchIndex } from "../accounts";
import type { AccountFilter, WorkbenchAccount } from "../../types";
import { accountListFixture } from "../../__tests__/list-pagination-fixture";

function filter(accounts: WorkbenchAccount[], criteria: AccountFilter): WorkbenchAccount[] {
  return filterAccountSearchIndex(createAccountSearchIndex(accounts), criteria);
}

const all: AccountFilter = { query: "", status: "all", group: "all" };

it("同一目录连续切换搜索和分组时返回完整匹配，不遗留上一次筛选", () => {
  const accounts = accountListFixture(2);
  accounts[1].groups = [{ id: "8", name: "团队" }];
  const index = createAccountSearchIndex(accounts);
  expect(filterAccountSearchIndex(index, { ...all, query: "account-1@" })).toEqual([accounts[0]]);
  expect(filterAccountSearchIndex(index, { ...all, group: "8" })).toEqual([accounts[1]]);
  expect(filterAccountSearchIndex(index, { ...all, query: "absent" })).toEqual([]);
  expect(filterAccountSearchIndex(index, all)).toEqual(accounts);
});

it("空目录和空白搜索保持原顺序与账号对象", () => {
  expect(filter([], all)).toEqual([]);
  const accounts = accountListFixture(2).reverse();
  const result = filter(accounts, { ...all, query: "  " });
  expect(result).toEqual(accounts);
  expect(result[0]).toBe(accounts[0]);
});

it.each(["TEAM", "OWNER@", "41", "团队组", "name owner", "😀"])(
  "搜索 %s 覆盖名称邮箱ID及分组，保留跨字段和Unicode匹配",
  (query) => {
    const accounts = accountListFixture(2);
    accounts[0] = {
      ...accounts[0],
      id: "41",
      name: "Team name",
      email: "owner@example.test",
      groups: [{ id: "7", name: "团队组😀" }],
    };
    expect(filter(accounts, { ...all, query: ` ${query} ` })).toEqual([accounts[0]]);
  },
);

it("状态分组与搜索同时生效，分组按稳定ID匹配", () => {
  const accounts = accountListFixture(3);
  accounts[0].groups = [{ id: "7", name: "共享组" }];
  accounts[1].groups = [{ id: "8", name: "共享组" }];
  accounts[2].groups = [{ id: "7", name: "共享组" }];
  accounts[2].schedulable = false;
  expect(filter(accounts, { query: "共享", status: "active", group: "7" })).toEqual([accounts[0]]);
  expect(filter(accounts, { query: "共享", status: "inactive", group: "7" })).toEqual([
    accounts[2],
  ]);
  expect(filter(accounts, { ...all, group: "共享组" })).toEqual([]);
});

it.each([
  ["ACTIVE", true, "active"],
  ["active", false, "inactive"],
  ["disabled", true, "inactive"],
  ["manually_disabled", true, "inactive"],
  ["inactive", true, "inactive"],
  ["", true, "unknown"],
  ["UNKNOWN", true, "unknown"],
  ["rate_limited", true, "error"],
] as const)("状态 %s 调度 %s 按 %s 筛选", (status, schedulable, expected) => {
  const accounts = accountListFixture(1);
  accounts[0] = { ...accounts[0], status, schedulable };
  expect(filter(accounts, { ...all, status: expected })).toEqual(accounts);
});

it("超长名称尾部可搜索，刷新后的名称和分组取代原内容", () => {
  const accounts = accountListFixture(1);
  accounts[0].name = "名称".repeat(2000) + "尾部";
  expect(filter(accounts, { ...all, query: "尾部" })).toEqual(accounts);
  const updated = [{ ...accounts[0], name: "新的名称", groups: [{ id: "9", name: "新分组" }] }];
  expect(filter(updated, { ...all, query: "尾部" })).toEqual([]);
  expect(filter(updated, { ...all, query: "新分组", group: "9" })).toEqual(updated);
});
