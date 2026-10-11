import { expect, it } from "vitest";
import type { AccountModelSyncAccount } from "@/api";
import { includedGroupModelKeys } from "../model-sync-selection";
import type { ModelSyncPlatformGroup } from "../model-sync-groups";

const accounts: AccountModelSyncAccount[] = [
  {
    account_id: "a",
    account_name: "同名账号",
    models: ["GPT", "manual"],
    enabled_models: ["gpt", "stale"],
    probe_model: "",
  },
  {
    account_id: "b",
    account_name: "同名账号",
    models: ["other"],
    enabled_models: ["other"],
    probe_model: "",
  },
  { account_id: "c", account_name: "未启用账号", models: ["manual"], probe_model: "" },
];
const group: ModelSyncPlatformGroup["groups"][number] = {
  key: "group",
  scopeKey: "openai\u0000group",
  label: "分组",
  accountCount: 2,
  accountIds: ["a", "c"],
  models: ["GPT", "manual", "other"].map((model) => ({
    model,
    accountCount: 1,
    supportingAccounts: [],
  })),
};

it("分组包含同名账号时仅按稳定 ID 继承本组启用模型并忽略目录外旧值", () => {
  expect(includedGroupModelKeys(accounts, group)).toEqual(new Set(["gpt"]));
});

it("本组手动包含和排除同时存在时排除优先，模型名大小写不影响默认勾选", () => {
  expect(
    includedGroupModelKeys(accounts, group, new Set(["gpt"]), new Set(["manual", "gpt"])),
  ).toEqual(new Set(["manual"]));
});

it("切换至其他分组后按新成员计算勾选，不继承前一组的启用模型", () => {
  expect(includedGroupModelKeys(accounts, { ...group, accountIds: ["b"] })).toEqual(
    new Set(["other"]),
  );
});

it("没有当前分组或模型目录为空时不生成勾选", () => {
  expect(includedGroupModelKeys(accounts, undefined)).toEqual(new Set());
  expect(includedGroupModelKeys(accounts, { ...group, models: [] })).toEqual(new Set());
});
