import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactElement } from "react";

import { api, type TrafficRankingSort } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { SearchField } from "@/components/data-table/search-field";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { useClientPagination } from "@/hooks/use-client-pagination";
import {
  orderedDictionaryOptions,
  trafficRankingSortOptions,
  trafficTimeRangeOptions,
} from "@/lib/domain-dictionaries";

import {
  normalizeTrafficSearchQuery,
  trafficAccountMatchesNormalized,
} from "../lib/traffic-ranking";
import { TrafficRankingTable } from "./traffic-ranking-table";

type TimeRange = "1h" | "6h" | "24h" | "7d" | "30d";

const timeRanges: ReadonlyArray<{ value: TimeRange; label: string }> = trafficTimeRangeOptions;
const rankingSorts: ReadonlyArray<{ value: TrafficRankingSort; label: string }> =
  trafficRankingSortOptions;

const pageSizes = [10, 20, 50, 100];

export function TrafficRankingPage(): ReactElement {
  const [timeRange, setTimeRange] = useState<TimeRange>("24h");
  const [group, setGroup] = useState<string | null>(null);
  const [platform, setPlatform] = useState("all");
  const [sortBy, setSortBy] = useState<TrafficRankingSort>("traffic");
  const [search, setSearch] = useState("");
  const groups = useQuery({ queryKey: ["groups"], queryFn: ({ signal }) => api.groups(signal) });
  const orderedGroups = useDictionaryOrder("group", groups.data ?? [], (group) => group.id ?? "");
  const platformDictionary = useQuery({
    queryKey: ["dictionaries", "platform"],
    queryFn: () => api.dictionaries("platform"),
  });
  const ranking = useQuery({
    queryKey: ["traffic-ranking", timeRange, group, sortBy],
    queryFn: (context) =>
      api.trafficRanking(
        {
          timeRange,
          group: group ?? undefined,
          sortBy,
        },
        context.signal,
      ),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
  const visibleAccounts = useMemo(() => {
    const normalizedSearch = normalizeTrafficSearchQuery(search);
    const normalizedPlatform = platform === "all" ? null : platform.trim().toLocaleLowerCase();
    return (ranking.data?.accounts ?? []).filter(
      (row) =>
        trafficAccountMatchesNormalized(row, normalizedSearch) &&
        (normalizedPlatform === null ||
          row.platform?.trim().toLocaleLowerCase() === normalizedPlatform),
    );
  }, [ranking.data?.accounts, platform, search]);
  const platformOptions = orderedDictionaryOptions(platformDictionary.data?.items);
  const pagination = useClientPagination(visibleAccounts);
  useEffect(
    () => pagination.setCurrentPage(1),
    [pagination.setCurrentPage, search, timeRange, group, platform, sortBy],
  );

  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="OPERATIONS / TRAFFIC"
        title="流量排行"
        description=""
        action={
          <PageActions>
            <RefreshButton
              pending={ranking.isFetching}
              ariaLabel="刷新流量排行"
              onClick={() => void ranking.refetch()}
            />
          </PageActions>
        }
      />
      {ranking.error ? <QueryErrorToast error={ranking.error} fallback="流量排行读取失败" /> : null}
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        <TableFilterToolbar>
          <div className="flex w-full min-w-0 flex-wrap items-center gap-2 sm:w-auto sm:flex-1">
            <SearchField value={search} onChange={setSearch} placeholder="搜索账号" />
            <FilterMenu
              label="时间范围"
              options={timeRanges.map((option) => option.value)}
              value={timeRange}
              onValueChange={(value) => value && setTimeRange(value)}
              optionLabel={(value) =>
                timeRanges.find((option) => option.value === value)?.label ?? value
              }
              clearable={false}
            />
            <FilterMenu
              label="账号分组"
              dictionaryKind="group"
              options={orderedGroups.map((item) => item.name)}
              value={group}
              onValueChange={setGroup}
            />
            <FilterMenu
              label="平台"
              dictionaryKind="platform"
              options={platformOptions.map((option) => option.value)}
              value={platform === "all" ? null : platform}
              onValueChange={(value) => setPlatform(value ?? "all")}
              optionLabel={(value) =>
                platformOptions.find((option) => option.value === value)?.label ?? value
              }
            />
          </div>
          <FilterMenu
            className="sm:ml-auto"
            label="排行维度"
            options={rankingSorts.map((option) => option.value)}
            value={sortBy}
            onValueChange={(value) => value && setSortBy(value)}
            optionLabel={(value) =>
              rankingSorts.find((option) => option.value === value)?.label ?? value
            }
            clearable={false}
          />
        </TableFilterToolbar>

        <DataTablePanel className="flex-1">
          <div className="flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b px-3 py-2.5">
            <h2 className="text-sm font-medium">账号排行</h2>
            <p className="text-muted-foreground text-xs">名次与占比按时间范围、分组统计</p>
          </div>
          {ranking.isLoading ? (
            <PageLoadingSkeleton label="流量排行加载中" fill framed={false} />
          ) : (
            <>
              <TrafficRankingTable
                rows={pagination.visibleItems}
                bucket={ranking.data?.bucket ?? "hour"}
                sortBy={sortBy}
                failed={!ranking.data && ranking.isError}
                pending={ranking.isFetching}
                onRetry={() => void ranking.refetch()}
              />
              <DataTablePagination
                currentPage={pagination.currentPage}
                totalPages={pagination.totalPages}
                totalItems={visibleAccounts.length}
                pageSize={pagination.pageSize}
                pageSizes={pageSizes}
                onPageChange={pagination.setCurrentPage}
                onPageSizeChange={pagination.setPageSize}
              />
            </>
          )}
        </DataTablePanel>
      </div>
    </PageLayout>
  );
}
