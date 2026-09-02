import { FilterMenu } from "@/components/data-table/filter-menu";
import { accountPoolFilters, type AccountPoolFilter } from "@/features/accounts/lib/account-pool";

type SelectableAccountPoolFilter = Exclude<AccountPoolFilter, "all">;

export const accountStatusFilterOptions = accountPoolFilters.filter(
  (filter): filter is { value: SelectableAccountPoolFilter; label: string } =>
    filter.value !== "all",
);

export function AccountStatusFilter(props: {
  value: AccountPoolFilter;
  onValueChange: (value: AccountPoolFilter) => void;
}) {
  return (
    <FilterMenu
      label="状态"
      options={accountStatusFilterOptions.map((filter) => filter.value)}
      value={props.value === "all" ? null : props.value}
      onValueChange={(value) => props.onValueChange(value ?? "all")}
      optionLabel={(value) =>
        accountStatusFilterOptions.find((filter) => filter.value === value)?.label ?? value
      }
    />
  );
}
