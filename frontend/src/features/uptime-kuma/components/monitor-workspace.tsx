import { useMemo, useState } from "react";
import type { KumaMonitor } from "@/api";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Table, TableBody, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { SearchField } from "@/components/data-table/search-field";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { monitorStatus, actionLabels, monitorStatusOptions } from "../constants";
import { MonitorDetail } from "./monitor-detail";
import { MonitorTreeRow } from "./monitor-tree-row";
import { buildMonitorTree, filterMonitorTree } from "../lib/monitor-tree";

type Action = keyof typeof actionLabels;
export function MonitorWorkspace(props: {
  monitors: KumaMonitor[];
  management: boolean;
  disabled: boolean;
  onEdit: (monitor: KumaMonitor) => void;
  onAction: (monitor: KumaMonitor, action: Action) => void;
}) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("all");
  const [detailKey, setDetailKey] = useState<string | null>(null);
  const [groupFilter, setGroupFilter] = useState("all");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const tree = useMemo(() => buildMonitorTree(props.monitors), [props.monitors]);
  const groups = tree.filter((row) => row.monitor.type === "group" || row.hasChildren);
  const filtering = !!search.trim() || filter !== "all" || groupFilter !== "all";
  const visible = filterMonitorTree(tree, {
    search,
    status: filter,
    group: groupFilter,
    management: props.management,
    collapsed,
  });
  const pagination = useClientPagination(visible);
  const toggleGroup = (key: string): void => {
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
    pagination.setCurrentPage(1);
  };
  const detail = props.monitors.find((monitor) => monitor.key === detailKey);
  const healthy = props.monitors.filter(
    (monitor) => monitorStatus(monitor, props.management) === "正常",
  ).length;
  const down = props.monitors.filter(
    (monitor) => monitorStatus(monitor, props.management) === "故障",
  ).length;
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
      <TableFilterToolbar aria-label="监控筛选">
        <SearchField
          placeholder="搜索监控项或地址"
          value={search}
          onChange={(value) => {
            setSearch(value);
            pagination.setCurrentPage(1);
          }}
        />
        <div className="w-full sm:w-40">
          <Select
            value={filter}
            onValueChange={(value) => {
              setFilter(value ?? "all");
              pagination.setCurrentPage(1);
            }}
            itemToStringLabel={(value) => (value === "all" ? "全部状态" : value)}
          >
            <SelectTrigger aria-label="筛选监控状态">
              <SelectValue />
            </SelectTrigger>
            <SelectContent searchable={false}>
              <SelectItem value="all">全部状态</SelectItem>
              {monitorStatusOptions.map((status) => (
                <SelectItem key={status} value={status}>
                  {status}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {props.management && (
          <div className="w-full sm:w-48">
            <Select
              value={groupFilter}
              onValueChange={(value) => {
                setGroupFilter(value ?? "all");
                pagination.setCurrentPage(1);
              }}
              itemToStringLabel={(value) => {
                if (value === "all") return "全部分组";
                if (value === "ungrouped") return "未分组";
                return (
                  groups.find((row) => String(row.monitor.id) === value)?.monitor.name ??
                  "分组已移除"
                );
              }}
            >
              <SelectTrigger aria-label="筛选监控分组">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部分组</SelectItem>
                <SelectItem value="ungrouped">未分组</SelectItem>
                {groups.map((row) => (
                  <SelectItem key={row.monitor.key} value={String(row.monitor.id)}>
                    {[...row.ancestors.map((item) => item.name), row.monitor.name].join(" / ")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <p className="text-muted-foreground text-sm sm:ml-auto">
          共 {props.monitors.length} 项 · 正常 {healthy} · 故障 {down}
        </p>
      </TableFilterToolbar>
      {!props.management && (
        <p className="text-muted-foreground text-sm">
          当前仅查看指标；管理账号请在侧栏「接入配置」中设置。暂停项可能不会出现在指标列表中。
        </p>
      )}
      <DataTablePanel role="region" aria-label="监控项列表" className="flex-1">
        <Table
          aria-label="监控项"
          className="min-w-[78rem]"
          containerClassName="min-h-0 flex-1 overflow-auto"
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-64">监控项名称</TableHead>
              <TableHead className="w-40">所属分组</TableHead>
              <TableHead className="w-24">状态</TableHead>
              <TableHead className="w-24">类型</TableHead>
              <TableHead className="w-64">监控地址</TableHead>
              <TableHead className="w-24">检测间隔</TableHead>
              <TableHead className="w-28">响应时间</TableHead>
              <TableHead className="w-32">24 小时在线率</TableHead>
              {props.management && <TableHead className="w-28 text-right">操作</TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.map((row) => (
              <MonitorTreeRow
                key={row.monitor.key}
                row={row}
                expanded={filtering || !collapsed.has(row.monitor.key)}
                filtering={filtering}
                management={props.management}
                disabled={props.disabled}
                onToggle={toggleGroup}
                onDetail={setDetailKey}
                onEdit={props.onEdit}
                onAction={props.onAction}
              />
            ))}
            {visible.length === 0 && (
              <TableEmptyState columns={props.management ? 9 : 8}>
                {props.monitors.length === 0 ? "暂无监控项" : "没有匹配的监控项"}
              </TableEmptyState>
            )}
          </TableBody>
        </Table>
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={visible.length}
          pageSize={pagination.pageSize}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      </DataTablePanel>
      {detail && (
        <MonitorDetail
          monitor={detail}
          management={props.management}
          onClose={() => setDetailKey(null)}
        />
      )}
    </div>
  );
}
