import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { api, type TrafficRankingSort } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { SearchField } from "@/components/data-table/search-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";

import {
  formatTrafficCount,
  formatTrafficLatency,
  formatTrafficPercent,
  trafficAccountMatches,
  trafficStabilityLabel,
} from "../lib/traffic-ranking";

type TimeRange = "1h" | "6h" | "24h" | "7d" | "30d";

const timeRanges: Array<{ value: TimeRange; label: string }> = [
  { value: "1h", label: "最近 1 小时" },
  { value: "6h", label: "最近 6 小时" },
  { value: "24h", label: "最近 24 小时" },
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
];

const rankingSorts: Array<{ value: TrafficRankingSort; label: string }> = [
  { value: "traffic", label: "按流量" },
  { value: "stability", label: "按稳定性" },
  { value: "success_rate", label: "按成功率" },
  { value: "latency", label: "按 P95 延迟" },
];

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
  return (
    <div className="space-y-2 p-3" aria-label="流量排行加载中">
      {Array.from({ length: 6 }, (_, index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
}

export function TrafficRankingPage() {
  const [timeRange, setTimeRange] = useState<TimeRange>("24h");
  const [group, setGroup] = useState("all");
  const [sortBy, setSortBy] = useState<TrafficRankingSort>("traffic");
  const [search, setSearch] = useState("");
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
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
    () => (ranking.data?.accounts ?? []).filter((row) => trafficAccountMatches(row, search)),
    [ranking.data?.accounts, search],
  );
  const pagination = useClientPagination(visibleAccounts);
  const timeRangeLabel =
    timeRanges.find((option) => option.value === timeRange)?.label ?? timeRange;
  const sortByLabel = rankingSorts.find((option) => option.value === sortBy)?.label ?? sortBy;

  useEffect(
    () => pagination.setCurrentPage(1),
    [pagination.setCurrentPage, search, timeRange, group, sortBy],
  );

  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="OPERATIONS / TRAFFIC"
        title="流量排行"
        description=""
        action={
          <PageActions>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="outline"
                    size="icon-sm"
                    aria-label="刷新流量排行"
                    disabled={ranking.isFetching}
                    onClick={() => void ranking.refetch()}
                  />
                }
              >
                <RefreshCw className={ranking.isFetching ? "animate-spin" : undefined} />
              </TooltipTrigger>
              <TooltipContent>刷新流量排行</TooltipContent>
            </Tooltip>
          </PageActions>
        }
      />
      {ranking.error ? <QueryErrorToast error={ranking.error} fallback="流量排行读取失败" /> : null}
      <div className="flex h-full min-h-0 flex-col gap-3">
        <TableFilterToolbar>
          <SearchField value={search} onChange={setSearch} placeholder="搜索账号" />
          <Select value={timeRange} onValueChange={(value) => setTimeRange(value as TimeRange)}>
            <SelectTrigger appearance="faceted" size="sm" className="w-36" aria-label="时间范围">
              <SelectValue>{timeRangeLabel}</SelectValue>
            </SelectTrigger>
            <SelectContent appearance="faceted">
              {timeRanges.map((option) => (
                <SelectItem appearance="faceted" key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={group} onValueChange={(value) => setGroup(value ?? "all")}>
            <SelectTrigger appearance="faceted" size="sm" className="w-36" aria-label="账号分组">
              <SelectValue>{group === "all" ? "全部分组" : group}</SelectValue>
            </SelectTrigger>
            <SelectContent appearance="faceted">
              <SelectItem appearance="faceted" value="all">
                全部分组
              </SelectItem>
              {(groups.data ?? []).map((item) => (
                <SelectItem appearance="faceted" key={item.name} value={item.name}>
                  {item.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={sortBy} onValueChange={(value) => setSortBy(value as TrafficRankingSort)}>
            <SelectTrigger appearance="faceted" size="sm" className="w-36" aria-label="排行维度">
              <SelectValue>{sortByLabel}</SelectValue>
            </SelectTrigger>
            <SelectContent appearance="faceted">
              {rankingSorts.map((option) => (
                <SelectItem appearance="faceted" key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </TableFilterToolbar>

        <DataTablePanel className="flex-1">
          {ranking.isLoading ? (
            <TrafficRankingSkeleton />
          ) : (
            <>
              <div className="min-h-0 flex-1 overflow-hidden">
                <Table containerClassName="h-full overflow-auto" className="min-w-[980px]">
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
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pagination.visibleItems.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={8} className="text-muted-foreground h-28 text-center">
                          当前范围没有匹配的账号流量
                        </TableCell>
                      </TableRow>
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
