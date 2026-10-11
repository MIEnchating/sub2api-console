import { expect, it } from "vitest";
import { accountListFixture } from "../../__tests__/list-pagination-fixture";
import { collectAccountGroups } from "../account-groups";

it("空目录和无分组账号不生成额外分组选项", () => {
  expect(collectAccountGroups([]).size).toBe(0);
  expect(collectAccountGroups(accountListFixture(1)).size).toBe(0);
});

it("重复ID保持首次出现位置并使用最后一份名称，同名不同ID分别保留", () => {
  const accounts = accountListFixture(2);
  accounts[0].groups = [
    { id: "7", name: "旧名称" },
    { id: "3", name: "同名" },
  ];
  accounts[1].groups = [
    { id: "7", name: "同名" },
    { id: "9", name: "" },
  ];
  expect(Array.from(collectAccountGroups(accounts).values())).toEqual([
    { id: "7", name: "同名" },
    { id: "3", name: "同名" },
    { id: "9", name: "" },
  ]);
});

it("目录替换时只包含新分组且不修改之前的目录", () => {
  const accounts = accountListFixture(1);
  accounts[0].groups = [{ id: "7", name: "原组" }];
  const previous = collectAccountGroups(accounts);
  const updated = collectAccountGroups([{ ...accounts[0], groups: [{ id: "8", name: "新组" }] }]);
  expect(Array.from(updated.keys())).toEqual(["8"]);
  expect(Array.from(previous.values())).toEqual([{ id: "7", name: "原组" }]);
  expect(accounts[0].groups).toEqual([{ id: "7", name: "原组" }]);
});
