import { bench, describe } from "vitest";
import { accountListFixture } from "../../__tests__/list-pagination-fixture";
import type { AccountFilter, WorkbenchAccount } from "../../types";
import { accountStatus, createAccountSearchIndex, filterAccountSearchIndex } from "../accounts";

// Preserved pre-index path for before/after measurement, never used as a test oracle.
function previousFilter(accounts: WorkbenchAccount[], filter: AccountFilter): WorkbenchAccount[] {
  return accounts.filter((account) => {
    const words = [
      account.name,
      account.email,
      account.id,
      ...account.groups.map((group) => group.name),
    ]
      .join(" ")
      .toLowerCase();
    return (
      words.includes(filter.query.trim().toLowerCase()) &&
      (filter.status === "all" || accountStatus(account) === filter.status) &&
      (filter.group === "all" || account.groups.some((group) => group.id === filter.group))
    );
  });
}

for (const size of [20, 1000, 10000]) {
  describe(`${size} accounts`, () => {
    const accounts = accountListFixture(size);
    for (const account of accounts)
      account.groups = [
        { id: "7", name: "团队分组" },
        { id: "8", name: "shared-group" },
      ];
    const index = createAccountSearchIndex(accounts);
    const filters: AccountFilter[] = [
      { query: "", status: "all", group: "all" },
      { query: "account-1@", status: "all", group: "all" },
      { query: "ACCOUNT-", status: "active", group: "7" },
      { query: "absent", status: "all", group: "all" },
    ];
    const expectedCount = size * 2 + 1;
    const options = { time: 750, warmupTime: 250 };
    bench(
      "previous / four filters",
      () => {
        let count = 0;
        for (const filter of filters) count += previousFilter(accounts, filter).length;
        if (count !== expectedCount) throw new Error("unexpected result count");
      },
      options,
    );
    bench(
      "indexed / four filters",
      () => {
        let count = 0;
        for (const filter of filters) count += filterAccountSearchIndex(index, filter).length;
        if (count !== expectedCount) throw new Error("unexpected result count");
      },
      options,
    );
    bench(
      "build + four filters",
      () => {
        const fresh = createAccountSearchIndex(accounts);
        let count = 0;
        for (const filter of filters) count += filterAccountSearchIndex(fresh, filter).length;
        if (count !== expectedCount) throw new Error("unexpected result count");
      },
      options,
    );
    bench(
      "build + initial empty filter",
      () => {
        if (
          filterAccountSearchIndex(createAccountSearchIndex(accounts), filters[0]).length !== size
        )
          throw new Error("unexpected result count");
      },
      options,
    );
    bench(
      "previous / initial empty filter",
      () => {
        if (previousFilter(accounts, filters[0]).length !== size)
          throw new Error("unexpected result count");
      },
      options,
    );
  });
}
