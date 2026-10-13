import type { KeyCleanupPreview } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useClientPagination } from "@/hooks/use-client-pagination";

import type { CleanupResultItem } from "./onboarding-key-cleanup-dialog";

const cleanupStatusLabels = { deleted: "已删除", skipped: "已跳过", failed: "失败" } as const;
const cleanupStatusVariants = {
  deleted: "success",
  skipped: "warning",
  failed: "destructive",
} as const;

export function KeyCleanupPreviewTable(props: { keys: KeyCleanupPreview["keys"] }) {
  const pagination = useClientPagination(props.keys);
  return (
    <DataTablePanel>
      <Table
        aria-label="待清理 Key"
        className="min-w-[680px] table-fixed"
        containerClassName="max-h-[24rem] overflow-auto"
      >
        <TableHeader className="sticky top-0 z-10">
          <TableRow>
            <TableHead className="w-[30%]">Key 名称</TableHead>
            <TableHead className="w-[28%]">Key ID</TableHead>
            <TableHead className="w-[24%]">上游分组</TableHead>
            <TableHead className="w-[18%]">状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pagination.visibleItems.map((key) => (
            <TableRow key={key.key_id}>
              <TableCell className="font-medium">{key.name || `Key ${key.key_id}`}</TableCell>
              <TableCell>{key.key_id}</TableCell>
              <TableCell>{key.group_id || "未返回"}</TableCell>
              <TableCell>{key.status || "未返回"}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <DataTablePagination
        currentPage={pagination.currentPage}
        totalPages={pagination.totalPages}
        totalItems={props.keys.length}
        pageSize={pagination.pageSize}
        onPageChange={pagination.setCurrentPage}
        onPageSizeChange={pagination.setPageSize}
      />
    </DataTablePanel>
  );
}

export function KeyCleanupResultsTable(props: { items: CleanupResultItem[] }) {
  const pagination = useClientPagination(props.items);
  return (
    <DataTablePanel>
      <Table
        aria-label="Key 清理结果"
        className="min-w-[680px] table-fixed"
        containerClassName="max-h-[22rem] overflow-auto"
      >
        <TableHeader className="sticky top-0 z-10">
          <TableRow>
            <TableHead className="w-[30%]">Key 名称</TableHead>
            <TableHead className="w-[28%]">Key ID</TableHead>
            <TableHead className="w-[16%]">结果</TableHead>
            <TableHead className="w-[26%]">说明</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pagination.visibleItems.map((item) => (
            <TableRow key={item.keyId}>
              <TableCell className="font-medium">{item.name || `Key ${item.keyId}`}</TableCell>
              <TableCell>{item.keyId}</TableCell>
              <TableCell>
                <Badge variant={cleanupStatusVariants[item.status]}>
                  {cleanupStatusLabels[item.status]}
                </Badge>
              </TableCell>
              <TableCell>{item.reason || "—"}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <DataTablePagination
        currentPage={pagination.currentPage}
        totalPages={pagination.totalPages}
        totalItems={props.items.length}
        pageSize={pagination.pageSize}
        onPageChange={pagination.setCurrentPage}
        onPageSizeChange={pagination.setPageSize}
      />
    </DataTablePanel>
  );
}
