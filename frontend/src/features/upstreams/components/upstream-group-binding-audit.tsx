import type { UpstreamGroupBindingAuditItem } from "@/api";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { StatusBadge } from "@/components/status-badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export type UpstreamGroupBindingAuditSummary = {
  present: number;
  missing: number;
  unknown: number;
};

export function summarizeUpstreamGroupBindings(
  items: UpstreamGroupBindingAuditItem[],
): UpstreamGroupBindingAuditSummary {
  const summary = { present: 0, missing: 0, unknown: 0 };
  for (const item of items) summary[item.status]++;
  return summary;
}

export function upstreamHasGroupBindingAuditIssue(items: UpstreamGroupBindingAuditItem[]): boolean {
  return items.some((item) => item.status !== "present");
}

function auditStatus(item: UpstreamGroupBindingAuditItem) {
  if (item.status === "present") {
    return <StatusBadge label="存在" variant="success" />;
  }
  if (item.status === "missing") {
    return <StatusBadge label="缺失" variant="danger" title={item.reason ?? undefined} />;
  }
  return <StatusBadge label="待确认" variant="warning" title={item.reason ?? undefined} />;
}

export function UpstreamGroupBindingAuditTable(props: { items: UpstreamGroupBindingAuditItem[] }) {
  return (
    <DataTablePanel className="h-full flex-1">
      <Table className="min-w-[720px]" containerClassName="min-h-0 flex-1 overflow-auto">
        <TableHeader>
          <TableRow>
            <TableHead className="w-28">核对结果</TableHead>
            <TableHead className="w-64">上游分组</TableHead>
            <TableHead>关联账号</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.items.length === 0 ? (
            <TableRow>
              <TableCell colSpan={3} className="text-muted-foreground h-32 text-center">
                该上游没有账号分组绑定
              </TableCell>
            </TableRow>
          ) : (
            props.items.map((item) => (
              <TableRow
                key={`${item.upstream_id}:${item.group_id ?? item.group_name}:${item.accounts
                  .map((account) => account.id)
                  .join(",")}`}
              >
                <TableCell>{auditStatus(item)}</TableCell>
                <TableCell overflowTooltip={false}>
                  <div className="grid min-w-0 gap-0.5">
                    <span className="truncate font-medium">{item.group_name}</span>
                    <span className="text-muted-foreground truncate text-xs">
                      {item.group_id ? `分组 ID：${item.group_id}` : "未记录分组 ID"}
                    </span>
                  </div>
                </TableCell>
                <TableCell overflowTooltip={false}>
                  <div className="flex min-w-0 flex-wrap gap-x-3 gap-y-1">
                    {item.accounts.map((account) => (
                      <span key={account.id} className="max-w-64 truncate text-sm">
                        {account.name || account.id}
                        {account.name ? (
                          <span className="text-muted-foreground ml-1">#{account.id}</span>
                        ) : null}
                      </span>
                    ))}
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </DataTablePanel>
  );
}
