import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import { api, type TrafficRankingSort } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { ContentRetry } from "@/components/content-retry";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { SearchField } from "@/components/data-table/search-field";
import { Badge } from "@/components/ui/badge";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { accountPlatformLabel } from "@/features/accounts/lib/account-labels";
import {
  orderedDictionaryOptions,
  trafficRankingSortOptions,
  trafficTimeRangeOptions,
} from "@/lib/domain-dictionaries";

import {
  formatTrafficCount,
  formatTrafficLatency,
  formatTrafficPercent,
  formatTrafficTokens,
  trafficAccountMatches,
  trafficStabilityLabel,
} from "../lib/traffic-ranking";

type TimeRange = "1h" | "6h" | "24h" | "7d" | "30d";

const timeRanges: ReadonlyArray<{ value: TimeRange; label: string }> = trafficTimeRangeOptions;
const rankingSorts: ReadonlyArray<{ value: TrafficRankingSort; label: string }> =
  trafficRankingSortOptions;

function latestTrafficLabel(value: string | null): string {
  if (value === null) return "-";
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function TrafficRankingSkeleton() {
  return <PageLoadingSkeleton label="流量排行加载中" fill framed={false} />;
}

export function TrafficRankingPage() {
  const [timeRange, setTimeRange] = useState<TimeRange>("24h");
  const [group, setGroup] = useState("all");
  const [platform, setPlatform] = useState("all");
  const [sortBy, setSortBy] = useState<TrafficRankingSort>("traffic");
  const [search, setSearch] = useState("");
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const platformDictionary = useQuery({
    queryKey: ["dictionaries", "platform"],
    queryFn: () => api.dictionaries("platform"),
  });
  const ranking = useQuery({
    queryKey: ["traffic-ranking", timeRange, group, sortBy],
    queryFn: () =>
      api.trafficRanking({
        timeRange,
        group: group === "all" ? undefined : group,
        sortBy,
      }),
    refetchInterval: 60_000,
  });
  const visibleAccounts = useMemo(
    () =>
      (ranking.data?.accounts ?? []).filter(
        (row) =>
          trafficAccountMatches(row, search) &&
          (platform === "all" || row.platform?.trim().toLocaleLowerCase() === platform),
      ),
    [ranking.data?.accounts, platform, search],
  );
  const platformOptions = useMemo(() => {
    const discovered = [
      ...new Set(
        (ranking.data?.accounts ?? [])
          .flatMap((row) => (row.platform ? [row.platform.trim().toLocaleLowerCase()] : []))
          .filter(Boolean),
      ),
    ].map((value) => ({ value, label: accountPlatformLabel(value) ?? value }));
    return orderedDictionaryOptions(platformDictionary.data?.items, discovered);
  }, [platformDictionary.data?.items, ranking.data?.accounts]);
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
      <div className="flex h-full min-h-0 flex-col gap-3">
        <TableFilterToolbar>
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
            options={(groups.data ?? []).map((item) => item.name)}
            value={group === "all" ? null : group}
            onValueChange={(value) => setGroup(value ?? "all")}
          />
          <FilterMenu
            label="平台"
            options={platformOptions.map((option) => option.value)}
            value={platform === "all" ? null : platform}
            onValueChange={(value) => setPlatform(value ?? "all")}
            optionLabel={(value) =>
              platformOptions.find((option) => option.value === value)?.label ?? value
            }
          />
          <FilterMenu
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
          {ranking.isLoading ? (
            <TrafficRankingSkeleton />
          ) : (
            <>
              <div className="min-h-0 flex-1 overflow-hidden">
                <Table containerClassName="h-full overflow-auto" className="min-w-[1120px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-14 text-center">排名</TableHead>
                      <TableHead className="w-72">账号</TableHead>
                      <TableHead className="w-32 text-right">请求 / 占比</TableHead>
                      <TableHead className="w-32 text-right">稳定性</TableHead>
                      <TableHead className="w-28 text-right">成功 / 失败</TableHead>
                      <TableHead className="w-40 text-right">平均 / P95</TableHead>
                      <TableHead className="w-28 text-right">活跃时段</TableHead>
                      <TableHead className="w-32 text-right">最后流量</TableHead>
                      <TableHead className="w-48 text-right">Token 用量</TableHead>
                      <TableHead className="w-48 text-right">缓存读取 / 写入</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {!ranking.data && ranking.isError && (
                      <TableEmptyState columns={10}>
                        <ContentRetry
                          pending={ranking.isFetching}
                          onRetry={() => void ranking.refetch()}
                        />
                      </TableEmptyState>
                    )}
                    {ranking.data && pagination.visibleItems.length === 0 ? (
                      <TableEmptyState columns={10}>当前范围没有匹配的账号流量</TableEmptyState>
                    ) : (
                      pagination.visibleItems.map((row) => {
                        const stability = trafficStabilityLabel(row.stability_score);
                        return (
                          <TableRow key={row.account_id}>
                            <TableCell className="text-center font-semibold">{row.rank}</TableCell>
                            <TableCell>
                              <div className="min-w-0">
                                <div className="truncate font-medium">{row.account_name}</div>
                                <div className="text-muted-foreground truncate text-xs">
                                  {[
                                    `#${row.account_id}`,
                                    row.groups.join("、") || "未分组",
                                    row.upstream_host || row.platform || "未标记上游",
                                  ].join(" · ")}
                                </div>
                              </div>
                            </TableCell>
                            <TableCell className="text-right font-medium">
                              {formatTrafficCount(row.requests)} /{" "}
                              {formatTrafficPercent(row.traffic_share)}
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="flex justify-end">
                                <Badge variant={stability.variant}>{stability.label}</Badge>
                              </div>
                              <div className="text-muted-foreground mt-1 text-xs">
                                {formatTrafficPercent(row.stability_score)}
                              </div>
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="font-medium">
                                {formatTrafficPercent(row.success_rate)}
                              </div>
                              <div className="text-muted-foreground text-xs">
                                {row.successful} / {row.failed}
                              </div>
                            </TableCell>
                            <TableCell className="text-right">
                              {formatTrafficLatency(row.average_latency_ms)} /{" "}
                              {formatTrafficLatency(row.p95_latency_ms)}
                            </TableCell>
                            <TableCell className="text-right">
                              {row.active_buckets} / {row.total_buckets}
                            </TableCell>
                            <TableCell className="text-right">
                              {latestTrafficLabel(row.latest_at)}
                            </TableCell>
                            <TableCell className="text-right">
                              {row.usage_available ? (
                                <>
                                  <div className="font-medium">
                                    {formatTrafficTokens(row.input_tokens)} /{" "}
                                    {formatTrafficTokens(row.output_tokens)}
                                  </div>
                                  <div className="text-muted-foreground text-xs">输入 / 输出</div>
                                </>
                              ) : (
                                <span className="text-muted-foreground">未提供</span>
                              )}
                            </TableCell>
                            <TableCell className="text-right">
                              {row.usage_available ? (
                                `${formatTrafficTokens(row.cache_read_tokens)} / ${formatTrafficTokens(row.cache_write_tokens)}`
                              ) : (
                                <span className="text-muted-foreground">未提供</span>
                              )}
                            </TableCell>
                          </TableRow>
                        );
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
              <DataTablePagination
                currentPage={pagination.currentPage}
                totalPages={pagination.totalPages}
                totalItems={visibleAccounts.length}
                pageSize={pagination.pageSize}
                pageSizes={[10, 20, 50, 100]}
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
