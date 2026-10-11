import { bench, describe } from "vitest";
import { accountListFixture } from "../../__tests__/list-pagination-fixture";
import { collectAccountGroups } from "../account-groups";
import type { WorkbenchAccount } from "../../types";

// Previous aggregation retained only as a benchmark reference, not a test oracle.
function previousGroups(accounts: WorkbenchAccount[]): WorkbenchAccount["groups"] {
  return Array.from(
    new Map(
      accounts.flatMap((account) => account.groups).map((group) => [group.id, group]),
    ).values(),
  );
}

for (const count of [20, 1000, 10000]) {
  describe(`${count} accounts / four groups each`, () => {
    const accounts = accountListFixture(count);
    for (const account of accounts) {
      account.groups = [
        { id: "1", name: "共同组" },
        { id: "2", name: "共享组" },
        { id: `team-${account.id}`, name: `团队 ${account.id}` },
        { id: `extra-${account.id}`, name: `扩展 ${account.id}` },
      ];
    }
    const options = { time: 750, warmupTime: 250 };
    bench(
      "previous aggregation",
      () => {
        if (previousGroups(accounts).length !== count * 2 + 2) throw new Error("incomplete groups");
      },
      options,
    );
    bench(
      "single traversal with values",
      () => {
        if (Array.from(collectAccountGroups(accounts).values()).length !== count * 2 + 2)
          throw new Error("incomplete groups");
      },
      options,
    );
  });
}
