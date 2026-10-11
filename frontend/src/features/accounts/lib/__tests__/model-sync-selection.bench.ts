// @vitest-environment node
import { bench, describe } from "vitest";
import type { AccountModelSyncAccount } from "@/api";
import type { ModelSyncPlatformGroup } from "../model-sync-groups";
import { includedGroupModelKeys } from "../model-sync-selection";

for (const count of [100, 1000]) {
  describe(`${count} 个账号、200 个发现模型的分组勾选`, () => {
    const models = Array.from({ length: 200 }, (_, index) =>
      index === 0 ? "enabled" : `discovered-${index}`,
    );
    const accounts: AccountModelSyncAccount[] = Array.from({ length: count }, (_, index) => ({
      account_id: String(index),
      account_name: `账号 ${index}`,
      models,
      enabled_models: ["enabled"],
      probe_model: "",
    }));
    const group: ModelSyncPlatformGroup["groups"][number] = {
      key: "group",
      scopeKey: "openai\u0000group",
      label: "分组",
      accountCount: count,
      accountIds: accounts.map((account) => account.account_id),
      models: models.map((model) => ({ model, accountCount: count, supportingAccounts: [] })),
    };
    bench(
      "重新计算启用状态",
      () => {
        const selected = includedGroupModelKeys(accounts, group);
        if (selected.size !== 1 || !selected.has("enabled")) throw new Error("分组勾选结果不一致");
      },
      { time: 1000, warmupTime: 300, iterations: 10 },
    );
  });
}
