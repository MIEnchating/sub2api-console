import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp } from "lucide-react";
import { SortableList, SortableItem } from "@/components/sortable-list";
import { useMemo, useState } from "react";
import { api, type DictionaryEntry, type DictionaryKind } from "@/api";
import { QueryErrorToast } from "@/components/query-error-toast";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { Skeleton } from "@/components/ui/skeleton";
import { ContentRetry } from "@/components/content-retry";
import { RefreshButton } from "@/components/refresh-button";
import { StatusBadge } from "@/components/status-badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { notifyOperationError } from "@/lib/operation-feedback";
const labels: Record<DictionaryKind, string> = {
  platform: "平台字典",
  group: "分组字典",
  account_type: "账号类型",
  upstream_type: "上游类型",
  auth_status: "鉴权状态",
  scheduling_strategy: "调度策略",
  task_status: "任务状态",
  account_status: "账号状态",
  alert_status: "告警状态",
  kuma_monitor_type: "监控类型",
};
const dictionaryKinds = Object.keys(labels) as DictionaryKind[];
type DictionaryOrder = { kind: DictionaryKind; items: DictionaryEntry[] };

export function DictionaryManagement() {
  const client = useQueryClient();
  const [kind, setKind] = useState<DictionaryKind>("platform");
  const [search, setSearch] = useState("");
  const [pendingOrder, setPendingOrder] = useState<DictionaryOrder | null>(null);
  const query = useQuery({
    queryKey: ["dictionaries", kind],
    queryFn: () => api.dictionaries(kind),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });
  const reorder = useMutation({
    mutationFn: (input: DictionaryOrder) =>
      api.reorderDictionaries(
        input.kind,
        input.items.map((item) => item.id),
      ),
    onMutate: async (input) => {
      const key = ["dictionaries", input.kind];
      await client.cancelQueries({ queryKey: key });
      const previous = client.getQueryData<{ items: DictionaryEntry[] }>(key);
      client.setQueryData(key, {
        items: input.items.map((item, index) => ({ ...item, sort_order: index })),
      });
      return { previous };
    },
    onSuccess: (_result, input) => {
      // Dependent pages refresh in the background; sorting only waits for its write.
      if (input.kind === "group") void client.invalidateQueries({ queryKey: ["groups"] });
    },
    onError: (error, input, context) => {
      if (context?.previous) client.setQueryData(["dictionaries", input.kind], context.previous);
      notifyOperationError(error, "字典排序失败");
    },
    onSettled: () => setPendingOrder(null),
  });
  const saving = pendingOrder !== null || reorder.isPending;
  const visibleItems = pendingOrder?.kind === kind ? pendingOrder.items : query.data?.items;
  const entries = useMemo(() => {
    const q = search.trim().toLocaleLowerCase();
    return (visibleItems ?? []).filter(
      (item) =>
        !q || `${item.name} ${item.value} ${item.description}`.toLocaleLowerCase().includes(q),
    );
  }, [visibleItems, search]);
  function saveOrder(items: DictionaryEntry[]): void {
    const input = { kind, items };
    // Commit the visible order with drag end, before the mutation's async cache update.
    setPendingOrder(input);
    reorder.mutate(input);
  }
  function move(entry: DictionaryEntry, direction: -1 | 1) {
    if (search.trim() || saving) return;
    const all = [...(query.data?.items ?? [])];
    const index = all.findIndex((item) => item.id === entry.id);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= all.length) return;
    [all[index], all[target]] = [all[target], all[index]];
    saveOrder(all);
  }
  function moveTo(entryId: string, targetId: string): void {
    if (entryId === targetId || Boolean(search.trim()) || saving) return;
    const all = [...(query.data?.items ?? [])];
    const from = all.findIndex((item) => item.id === entryId);
    const target = all.findIndex((item) => item.id === targetId);
    if (from < 0 || target < 0) return;
    const [item] = all.splice(from, 1);
    all.splice(target, 0, item);
    saveOrder(all);
  }
  return (
    <Card size="sm" className="h-full min-h-0 min-w-0">
      <CardHeader className="shrink-0 gap-3">
        <div>
          <CardTitle>字典管理</CardTitle>
          <CardDescription className="mt-1">
            平台、分组来自管理平台，其余为系统内置字典。
          </CardDescription>
        </div>
        <SegmentedControl className="w-full" role="tablist" aria-label="字典类型">
          {dictionaryKinds.map((value) => (
            <SegmentedControlItem
              key={value}
              id={`dictionary-tab-${value}`}
              role="tab"
              selected={kind === value}
              aria-controls={`dictionary-panel-${value}`}
              onClick={() => {
                setKind(value);
                setSearch("");
              }}
            >
              {labels[value]}
            </SegmentedControlItem>
          ))}
        </SegmentedControl>
      </CardHeader>
      <CardContent
        role="tabpanel"
        id={`dictionary-panel-${kind}`}
        aria-labelledby={`dictionary-tab-${kind}`}
        className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden"
      >
        {query.error ? <QueryErrorToast error={query.error} fallback="字典数据读取失败" /> : null}
        <TableFilterToolbar aria-label="字典筛选">
          <SearchField value={search} onChange={setSearch} placeholder="搜索名称、字典值或说明" />
          <span className="ml-auto text-sm text-muted-foreground">共 {entries.length} 项</span>
          <RefreshButton
            ariaLabel="刷新字典"
            pending={query.isFetching}
            disabled={saving}
            onClick={() => void query.refetch()}
          />
        </TableFilterToolbar>
        <SortableList
          key={kind}
          items={entries.map((entry) => ({ id: entry.id, label: entry.name }))}
          disabled={Boolean(search.trim()) || saving}
          onMove={(from, to) => moveTo(entries[from].id, entries[to].id)}
        >
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
            <Table
              actionColumn
              className="min-w-[760px]"
              containerClassName="min-h-0 flex-1 overflow-auto"
            >
              <TableHeader>
                <TableRow>
                  <TableHead className="w-16">序号</TableHead>
                  <TableHead>显示名称</TableHead>
                  <TableHead>字典值</TableHead>
                  <TableHead>说明</TableHead>
                  <TableHead className="w-24">状态</TableHead>
                  <TableHead className="w-28 text-right">排序</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {query.isLoading
                  ? Array.from({ length: 4 }, (_, index) => (
                      <TableRow key={index} aria-label="正在读取字典">
                        {Array.from({ length: 6 }, (_, column) => (
                          <TableCell key={column}>
                            <Skeleton className="h-4 w-3/4" />
                          </TableCell>
                        ))}
                      </TableRow>
                    ))
                  : null}
                {!query.isLoading && entries.length === 0 ? (
                  <TableEmptyState columns={6}>
                    {query.error ? (
                      <ContentRetry
                        onRetry={() => void query.refetch()}
                        pending={query.isFetching}
                      />
                    ) : (
                      "暂无字典数据"
                    )}
                  </TableEmptyState>
                ) : null}
                {!query.isLoading
                  ? entries.map((entry, index) => (
                      <SortableItem
                        key={entry.id}
                        id={entry.id}
                        label={entry.name}
                        disabled={Boolean(search.trim()) || saving}
                      >
                        {(sortable) => (
                          <TableRow
                            ref={sortable.ref}
                            style={sortable.style}
                            data-dragging={sortable.dragging || undefined}
                            className={
                              sortable.over ? "ring-1 ring-inset ring-primary/50" : undefined
                            }
                          >
                            <TableCell className="text-muted-foreground">{index + 1}</TableCell>
                            <TableCell className="font-medium">
                              <span className="inline-flex items-center gap-2">
                                {sortable.handle}
                                {entry.name}
                              </span>
                            </TableCell>
                            <TableCell>
                              <code className="text-xs">{entry.value}</code>
                            </TableCell>
                            <TableCell className="max-w-[240px] truncate text-muted-foreground">
                              {entry.description || "-"}
                            </TableCell>
                            <TableCell>
                              <StatusBadge
                                label={entry.enabled ? "启用" : "停用"}
                                variant={entry.enabled ? "success" : "neutral"}
                              />
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="flex justify-end gap-1">
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  aria-label={`上移${entry.name}`}
                                  disabled={Boolean(search.trim()) || index === 0 || saving}
                                  onClick={() => move(entry, -1)}
                                >
                                  <ArrowUp aria-hidden="true" />
                                </Button>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  aria-label={`下移${entry.name}`}
                                  disabled={
                                    Boolean(search.trim()) || index === entries.length - 1 || saving
                                  }
                                  onClick={() => move(entry, 1)}
                                >
                                  <ArrowDown aria-hidden="true" />
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        )}
                      </SortableItem>
                    ))
                  : null}
              </TableBody>
            </Table>
          </div>
        </SortableList>
      </CardContent>
    </Card>
  );
}
