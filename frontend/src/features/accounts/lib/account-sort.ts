import type { AccountStatus } from "@/api";

export type AccountSort =
  | "default"
  | "priority_asc"
  | "priority_desc"
  | "name_asc"
  | "name_desc"
  | "health_asc"
  | "health_desc"
  | "cost_asc"
  | "cost_desc"
  | "weight_asc"
  | "weight_desc"
  | "latency_asc"
  | "latency_desc";

export type AccountSortColumn = "priority" | "name" | "health" | "cost" | "weight" | "latency";
export type AccountSortDirection = "asc" | "desc";

const accountSortsByColumn: Record<AccountSortColumn, Record<AccountSortDirection, AccountSort>> = {
  priority: { asc: "priority_asc", desc: "priority_desc" },
  name: { asc: "name_asc", desc: "name_desc" },
  health: { asc: "health_asc", desc: "health_desc" },
  cost: { asc: "cost_asc", desc: "cost_desc" },
  weight: { asc: "weight_asc", desc: "weight_desc" },
  latency: { asc: "latency_asc", desc: "latency_desc" },
};

export function accountSortDirection(
  sort: AccountSort,
  column: AccountSortColumn,
): AccountSortDirection | null {
  if (sort === accountSortsByColumn[column].asc) return "asc";
  if (sort === accountSortsByColumn[column].desc) return "desc";
  return null;
}

export function nextAccountSort(sort: AccountSort, column: AccountSortColumn): AccountSort {
  const direction = accountSortDirection(sort, column);
  if (direction === "asc") return accountSortsByColumn[column].desc;
  if (direction === "desc") return "default";
  return accountSortsByColumn[column].asc;
}

const accountNameCollator = new Intl.Collator("zh-CN", {
  numeric: true,
  sensitivity: "base",
});

function numericValue(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined || value === "") return null;
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : null;
}

function compareNullableNumbers(
  left: number | null,
  right: number | null,
  direction: "asc" | "desc",
): number {
  if (left === null && right === null) return 0;
  if (left === null) return 1;
  if (right === null) return -1;
  return direction === "asc" ? left - right : right - left;
}

function compareAccountNames(left: AccountStatus, right: AccountStatus, direction = 1): number {
  const nameDifference = accountNameCollator.compare(left.name, right.name) * direction;
  if (nameDifference !== 0) return nameDifference;
  return accountNameCollator.compare(left.id, right.id) * direction;
}

function accountNumericSortValue(account: AccountStatus, sort: AccountSort): number | null {
  if (sort.startsWith("priority_")) {
    return numericValue(account.manual_priority ?? account.priority);
  }
  if (sort.startsWith("health_")) return numericValue(account.health_score);
  if (sort.startsWith("cost_")) return numericValue(account.multiplier);
  if (sort.startsWith("weight_")) return numericValue(account.weight);
  if (sort.startsWith("latency_")) return numericValue(account.ttfb_p95_ms);
  return null;
}

export function sortAccounts(
  accounts: readonly AccountStatus[],
  sort: AccountSort,
): AccountStatus[] {
  const sorted = [...accounts];
  if (sort === "default") return sorted;

  if (sort === "name_asc" || sort === "name_desc") {
    const direction = sort === "name_asc" ? 1 : -1;
    return sorted.sort((left, right) => compareAccountNames(left, right, direction));
  }

  const direction = sort.endsWith("_asc") ? "asc" : "desc";
  return sorted.sort((left, right) => {
    const difference = compareNullableNumbers(
      accountNumericSortValue(left, sort),
      accountNumericSortValue(right, sort),
      direction,
    );
    return difference || compareAccountNames(left, right);
  });
}
