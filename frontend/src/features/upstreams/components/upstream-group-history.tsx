import type { UpstreamGroupBindingAuditItem, UpstreamGroupChange } from "@/api";
import { Trash2, UserPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { upstreamBindingLabels } from "../constants";
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

type UpstreamGroupHistoryIdentity = {
  upstream_id: string;
  name: string;
  host: string;
};

export type UpstreamGroupHistoryProps = {
  rows: UpstreamGroupChange[];
  upstreams?: UpstreamGroupHistoryIdentity[];
  bindingAuditItems?: UpstreamGroupBindingAuditItem[];
  upstreamAvailable?: boolean;
  initialExpandedUpstreamID?: string;
  onClearUpstream?: (upstreamID: string, name: string) => void;
  onAddAccount?: (change: UpstreamGroupChange) => void;
  onDeleteAccounts?: (change: UpstreamGroupChange, accountIDs: string[]) => void;
};

function historyGroupKey(change: UpstreamGroupChange): string {
  return `${change.upstream_id}\x00${change.group_id}`;
}

export function latestHistoryGroupChanges(rows: UpstreamGroupChange[]): Set<number> {
  const latest = new Map<string, UpstreamGroupChange>();
  for (const row of rows) {
    const key = historyGroupKey(row);
    const current = latest.get(key);
    if (
      !current ||
      Date.parse(row.changed_at) > Date.parse(current.changed_at) ||
      (row.changed_at === current.changed_at && row.id > current.id)
    ) {
      latest.set(key, row);
    }
  }
  return new Set([...latest.values()].map((row) => row.id));
}

export function UpstreamGroupHistoryAction(props: {
  change: UpstreamGroupChange;
  latest: boolean;
  upstreamAvailable: boolean;
  bindingAuditItems?: UpstreamGroupBindingAuditItem[];
  onAddAccount?: (change: UpstreamGroupChange) => void;
  onDeleteAccounts?: (change: UpstreamGroupChange, accountIDs: string[]) => void;
}) {
  if (!props.latest) return null;
  if (props.change.change_type === "added") {
    const binding = props.bindingAuditItems?.find(
      (item) =>
        item.upstream_id === props.change.upstream_id && item.group_id === props.change.group_id,
    );
    if (binding && binding.account_count > 0) {
      return <Badge variant="secondary">{upstreamBindingLabels.bound}</Badge>;
    }
    if (!props.upstreamAvailable || !props.onAddAccount || binding?.status === "missing")
      return null;
    return (
      <Button
        variant="outline"
        aria-label={`向${props.change.group_name}添加账号`}
        onClick={() => props.onAddAccount?.(props.change)}
      >
        <UserPlus aria-hidden="true" />
        添加账号
      </Button>
    );
  }
  const audit = props.bindingAuditItems?.find(
    (item) =>
      item.upstream_id === props.change.upstream_id &&
      item.group_id === props.change.group_id &&
      item.status === "missing",
  );
  const accountIDs = [...new Set(audit?.accounts.map((account) => account.id) ?? [])];
  if (accountIDs.length === 0 || !props.onDeleteAccounts) return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-muted-foreground text-xs">{accountIDs.length} 个绑定账号</span>
      <Button
        variant="destructive"
        aria-label={`删除${props.change.group_name}的绑定账号`}
        onClick={() => props.onDeleteAccounts?.(props.change, accountIDs)}
      >
        <Trash2 aria-hidden="true" />
        删除绑定账号
      </Button>
    </div>
  );
}

export function UpstreamGroupHistory(props: UpstreamGroupHistoryProps) {
  if (props.upstreams)
    return <UpstreamGroupHistoryOverview {...props} upstreams={props.upstreams} />;
  return <UpstreamGroupHistoryDetails {...props} />;
}

function UpstreamGroupHistoryDetails(props: UpstreamGroupHistoryProps) {
  const pagination = useClientPagination(props.rows);
  const latestChanges = latestHistoryGroupChanges(props.rows);
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
            <TableHead className="w-44">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={5} className="text-muted-foreground h-32 text-center">
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
                  <TableCell overflowTooltip={false}>
                    <UpstreamGroupHistoryAction
                      change={row}
                      latest={latestChanges.has(row.id)}
                      upstreamAvailable={Boolean(props.upstreamAvailable)}
                      bindingAuditItems={props.bindingAuditItems}
                      onAddAccount={props.onAddAccount}
                      onDeleteAccounts={props.onDeleteAccounts}
                    />
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
