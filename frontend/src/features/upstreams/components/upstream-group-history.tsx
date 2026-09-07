import { useMemo } from "react";

import type { UpstreamGroupChange } from "@/api";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function changedAtText(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

export type UpstreamGroupHistoryIdentity = {
  upstream_id: string;
  name: string;
  host: string;
};

export function UpstreamGroupHistory(props: {
  rows: UpstreamGroupChange[];
  upstreams?: UpstreamGroupHistoryIdentity[];
}) {
  const pagination = useClientPagination(props.rows);
  const upstreamsByID = useMemo(
    () => new Map((props.upstreams ?? []).map((upstream) => [upstream.upstream_id, upstream])),
    [props.upstreams],
  );
  const showUpstream = props.upstreams !== undefined;
  const columnCount = showUpstream ? 4 : 3;
  return (
    <DataTablePanel className="h-full flex-1">
      <Table
        className={showUpstream ? "min-w-[760px]" : "min-w-[640px]"}
        containerClassName="min-h-0 flex-1 overflow-auto"
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-52">变化时间</TableHead>
            {showUpstream ? <TableHead className="w-56">上游</TableHead> : null}
            <TableHead className="w-28">变化</TableHead>
            <TableHead>上游分组</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="text-muted-foreground h-32 text-center">
                暂无上游分组变化记录
              </TableCell>
            </TableRow>
          ) : (
            pagination.visibleItems.map((row) => {
              const upstream = upstreamsByID.get(row.upstream_id);
              return (
                <TableRow key={row.id}>
                  <TableCell className="text-muted-foreground tabular-nums">
                    {changedAtText(row.changed_at)}
                  </TableCell>
                  {showUpstream ? (
                    <TableCell overflowTooltip={false}>
                      <div className="grid min-w-0 gap-0.5">
                        <span className="truncate font-medium">
                          {upstream?.name || upstream?.host || row.upstream_id}
                        </span>
                        {upstream?.host ? (
                          <span className="text-muted-foreground truncate text-xs">
                            {upstream.host}
                          </span>
                        ) : null}
                      </div>
                    </TableCell>
                  ) : null}
                  <TableCell>
                    <Badge variant={row.change_type === "added" ? "outline" : "destructive"}>
                      {row.change_type === "added" ? "添加" : "删除"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <span className="font-medium">{row.group_name}</span>
                    <span className="text-muted-foreground ml-2">#{row.group_id}</span>
                  </TableCell>
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>
      {props.rows.length > 0 ? (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={props.rows.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 50, 100]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      ) : null}
    </DataTablePanel>
  );
}
