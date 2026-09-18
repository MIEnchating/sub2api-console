import type { UpstreamGroupChange } from "@/api";
import { groupHistoryTime } from "../lib/group-history";
import { upstreamRateLabels } from "../lib/upstream-rate-labels";
import { UpstreamGroupHistoryOverview } from "./upstream-group-history-overview";
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

export type UpstreamGroupHistoryIdentity = {
  upstream_id: string;
  name: string;
  host: string;
};

export function UpstreamGroupHistory(props: {
  rows: UpstreamGroupChange[];
  upstreams?: UpstreamGroupHistoryIdentity[];
}) {
  if (props.upstreams)
    return <UpstreamGroupHistoryOverview rows={props.rows} upstreams={props.upstreams} />;
  return <UpstreamGroupHistoryDetails rows={props.rows} />;
}

function UpstreamGroupHistoryDetails(props: { rows: UpstreamGroupChange[] }) {
  const pagination = useClientPagination(props.rows);
  return (
    <DataTablePanel className="h-full flex-1">
      <Table className="min-w-[640px]" containerClassName="min-h-0 flex-1 overflow-auto">
        <TableHeader>
          <TableRow>
            <TableHead className="w-52">变化时间</TableHead>
            <TableHead className="w-28">变化</TableHead>
            <TableHead>上游分组</TableHead>
            <TableHead className="w-40" title="最近同步并按充值比例换算后的分组倍率">
              {upstreamRateLabels.effectiveRate}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={4} className="text-muted-foreground h-32 text-center">
                暂无上游分组变化记录
              </TableCell>
            </TableRow>
          ) : (
            pagination.visibleItems.map((row) => {
              return (
                <TableRow key={row.id}>
                  <TableCell className="text-muted-foreground tabular-nums">
                    {groupHistoryTime(row.changed_at)}
                  </TableCell>
                  <TableCell>
                    <Badge variant={row.change_type === "added" ? "outline" : "destructive"}>
                      {row.change_type === "added" ? "添加" : "删除"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <span className="font-medium">{row.group_name}</span>
                    <span className="text-muted-foreground ml-2">#{row.group_id}</span>
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {row.change_type === "added" ? (row.effective_rate ?? "未计算") : null}
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
