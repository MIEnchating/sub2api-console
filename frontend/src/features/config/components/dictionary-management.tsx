import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, GripVertical, RefreshCw } from "lucide-react";
import { useMemo, useState } from "react";
import { api, type DictionaryEntry, type DictionaryKind } from "@/api";
import { QueryErrorToast } from "@/components/query-error-toast";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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

export function DictionaryManagement() {
  const client = useQueryClient();
  const [kind, setKind] = useState<DictionaryKind>("platform");
  const [search, setSearch] = useState("");
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const query = useQuery({
    queryKey: ["dictionaries", kind],
    queryFn: () => api.dictionaries(kind),
  });
  const reorder = useMutation({
    mutationFn: (ids: string[]) => api.reorderDictionaries(kind, ids),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["dictionaries", kind] });
      if (kind === "group") await client.invalidateQueries({ queryKey: ["groups"] });
      if (kind === "platform") await client.invalidateQueries({ queryKey: ["accounts"] });
    },
    onError: (error) => notifyOperationError(error, "字典排序失败"),
  });
  const entries = useMemo(() => {
    const q = search.trim().toLocaleLowerCase();
    return (query.data?.items ?? []).filter(
      (item) =>
        !q || `${item.name} ${item.value} ${item.description}`.toLocaleLowerCase().includes(q),
    );
  }, [query.data?.items, search]);
  function move(entry: DictionaryEntry, direction: -1 | 1) {
    const all = [...(query.data?.items ?? [])];
    const index = all.findIndex((item) => item.id === entry.id);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= all.length) return;
    [all[index], all[target]] = [all[target], all[index]];
    reorder.mutate(all.map((item) => item.id));
  }
  function moveTo(entryId: string, targetId: string): void {
    if (entryId === targetId || Boolean(search.trim()) || reorder.isPending) return;
    const all = [...(query.data?.items ?? [])];
    const from = all.findIndex((item) => item.id === entryId);
    const target = all.findIndex((item) => item.id === targetId);
    if (from < 0 || target < 0) return;
    const [item] = all.splice(from, 1);
    all.splice(target, 0, item);
    reorder.mutate(all.map((value) => value.id));
  }
  return (
    <Card size="sm" className="h-full min-h-0 min-w-0">
      <CardHeader className="shrink-0 gap-3">
        <div>
          <CardTitle>字典管理</CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            字典值来自管理平台同步，本页仅调整展示顺序。
          </p>
        </div>
        <div
          className="inline-flex w-fit rounded-lg border border-border/70 bg-muted/40 p-1"
          role="tablist"
          aria-label="字典类型"
        >
          {dictionaryKinds.map((value) => (
            <Button
              key={value}
              variant={kind === value ? "secondary" : "ghost"}
              className="min-w-28 rounded-md px-4"
              role="tab"
              aria-selected={kind === value}
              onClick={() => {
                setKind(value);
                setSearch("");
              }}
            >
              {labels[value]}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden">
        {query.error ? <QueryErrorToast error={query.error} fallback="字典数据读取失败" /> : null}
        <TableFilterToolbar aria-label="字典筛选">
          <SearchField value={search} onChange={setSearch} placeholder="搜索名称、字典值或说明" />
          <span className="ml-auto text-sm text-muted-foreground">共 {entries.length} 项</span>
          <Button
            size="icon"
            variant="ghost"
            aria-label="刷新字典"
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" />
          </Button>
        </TableFilterToolbar>
        <div className="min-h-0 flex-1 overflow-auto rounded-md border">
          <Table className="min-w-[760px]">
            <TableHeader className="sticky top-0 z-10 bg-background">
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
              {query.isLoading ? (
                <TableRow>
                  <TableCell colSpan={6}>
                    <div className="py-12 text-center text-muted-foreground" role="status">
                      正在读取字典
                    </div>
                  </TableCell>
                </TableRow>
              ) : null}
              {!query.isLoading && entries.length === 0 ? (
                <TableEmptyState columns={6}>暂无管理平台字典数据</TableEmptyState>
              ) : null}
              {!query.isLoading
                ? entries.map((entry, index) => (
                    <TableRow
                      key={entry.id}
                      draggable={!search.trim() && !reorder.isPending}
                      aria-grabbed={draggingId === entry.id}
                      onDragStart={(event) => {
                        if (search.trim() || reorder.isPending) return;
                        setDraggingId(entry.id);
                        event.dataTransfer.effectAllowed = "move";
                        event.dataTransfer.setData("text/plain", entry.id);
                      }}
                      onDragEnd={() => setDraggingId(null)}
                      onDragOver={(event) => {
                        if (draggingId && draggingId !== entry.id) event.preventDefault();
                      }}
                      onDrop={(event) => {
                        event.preventDefault();
                        const sourceId = event.dataTransfer.getData("text/plain") || draggingId;
                        if (sourceId) moveTo(sourceId, entry.id);
                        setDraggingId(null);
                      }}
                    >
                      <TableCell className="text-muted-foreground">{index + 1}</TableCell>
                      <TableCell className="font-medium">
                        <span className="inline-flex items-center gap-2">
                          <GripVertical aria-hidden="true" className="text-muted-foreground" />
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
                        <span
                          className={entry.enabled ? "text-emerald-600" : "text-muted-foreground"}
                        >
                          {entry.enabled ? "启用" : "停用"}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button
                            size="icon"
                            variant="ghost"
                            aria-label={`上移${entry.name}`}
                            disabled={Boolean(search.trim()) || index === 0 || reorder.isPending}
                            onClick={() => move(entry, -1)}
                          >
                            <ArrowUp aria-hidden="true" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            aria-label={`下移${entry.name}`}
                            disabled={
                              Boolean(search.trim()) ||
                              index === entries.length - 1 ||
                              reorder.isPending
                            }
                            onClick={() => move(entry, 1)}
                          >
                            <ArrowDown aria-hidden="true" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                : null}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}
