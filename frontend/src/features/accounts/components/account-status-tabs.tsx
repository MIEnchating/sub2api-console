import { useQuery } from "@tanstack/react-query";
import { useMemo, type ReactElement } from "react";

import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { orderByDictionary } from "@/lib/dictionary-order";
import { accountPoolFilters, type AccountPoolFilter } from "@/features/accounts/lib/account-pool";

type SelectableAccountPoolFilter = Exclude<AccountPoolFilter, "all">;

export const accountStatusFilterOptions = accountPoolFilters.filter(
  (filter): filter is { value: SelectableAccountPoolFilter; label: string } =>
    filter.value !== "all",
);

const localStatusLabels = new Map(
  accountStatusFilterOptions.map((filter) => [filter.value, filter.label]),
);

export function AccountStatusFilter(props: {
  value: AccountPoolFilter;
  onValueChange: (value: AccountPoolFilter) => void;
}): ReactElement {
  const query = useQuery({
    queryKey: ["dictionaries", "account_status"],
    queryFn: () => api.dictionaries("account_status"),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });
  const { options, dictionaryNames } = useMemo(() => {
    const entries = query.data?.items ?? [];
    const registered = new Set(
      entries.filter((entry) => entry.enabled).map((entry) => entry.value),
    );
    return {
      options: orderByDictionary(
        accountStatusFilterOptions.filter((filter) => registered.has(filter.value)),
        entries,
        (filter) => filter.value,
      ),
      dictionaryNames: new Map(entries.map((entry) => [entry.value, entry.name])),
    };
  }, [query.data]);
  let emptyContent: ReactElement | undefined;
  if (!query.data && query.isPending) {
    emptyContent = <ContentLoading label="正在读取账号状态" compact />;
  } else if (!query.data && query.isError) {
    emptyContent = <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />;
  }
  return (
    <FilterMenu
      label="状态"
      options={options.map((filter) => filter.value)}
      emptyContent={emptyContent}
      value={props.value === "all" ? null : props.value}
      onValueChange={(value) => props.onValueChange(value ?? "all")}
      optionLabel={(value) => dictionaryNames.get(value) ?? localStatusLabels.get(value) ?? value}
    />
  );
}
