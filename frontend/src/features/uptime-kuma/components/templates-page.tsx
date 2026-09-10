import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type KumaTemplate } from "@/api";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { PageActions } from "@/components/page-actions";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { SearchField } from "@/components/data-table/search-field";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { notifyOperationError } from "@/lib/operation-feedback";
import { kumaTemplatesKey, monitorTypeLabels } from "../constants";
import type { TemplateValues } from "../lib/template-schema";
import { TemplateDialog } from "./template-dialog";

export function KumaTemplatesPage() {
  const client = useQueryClient();
  const query = useQuery({ queryKey: kumaTemplatesKey, queryFn: api.kumaTemplates, retry: false });
  const [search, setSearch] = useState("");
  const [editor, setEditor] = useState<{ item?: KumaTemplate } | null>(null);
  const [deleting, setDeleting] = useState<KumaTemplate | null>(null);
  const refresh = (): Promise<void> => client.invalidateQueries({ queryKey: kumaTemplatesKey });
  const save = useMutation({
    mutationFn: (value: TemplateValues) => api.saveKumaTemplate(editor?.item?.id, value),
    onSuccess: async () => {
      setEditor(null);
      toast.success("功能模板已保存");
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (item: KumaTemplate) => api.deleteKumaTemplate(item.id, item.revision),
    onSuccess: async () => {
      setDeleting(null);
      toast.success("功能模板已删除");
      await refresh();
    },
    onError: (error) => notifyOperationError(error, "模板删除失败"),
  });
  const pending = save.isPending || remove.isPending;
  const items = (query.data ?? []).filter((item) =>
    item.name.toLowerCase().includes(search.trim().toLowerCase()),
  );
  const pagination = useClientPagination(items);
  const disabled = pending || query.isFetching || query.isError;
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="Uptime Kuma"
        title="功能模板"
        description="复用监控参数、请求内容和鉴权设置"
        action={
          <PageActions>
            <RefreshButton
              pending={query.isFetching}
              disabled={pending}
              onClick={() => void refresh()}
            />
            <Button
              disabled={disabled}
              onClick={() => {
                save.reset();
                setEditor({});
              }}
            >
              <CirclePlus aria-hidden="true" />
              新增模板
            </Button>
          </PageActions>
        }
      />
      {query.error && <QueryErrorToast error={query.error} fallback="模板读取失败" />}
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        {query.isPending && <PageLoadingSkeleton label="正在读取功能模板…" variant="table" />}
        {query.data && (
          <>
            <TableFilterToolbar>
              <SearchField
                placeholder="搜索模板名称"
                value={search}
                onChange={(value) => {
                  setSearch(value);
                  pagination.setCurrentPage(1);
                }}
              />
            </TableFilterToolbar>
            <DataTablePanel className="flex-1">
              <Table
                aria-label="功能模板"
                className="min-w-[72rem]"
                containerClassName="min-h-0 flex-1 overflow-auto"
              >
                <TableHeader>
                  <TableRow>
                    <TableHead>模板名称</TableHead>
                    <TableHead>监控类型</TableHead>
                    <TableHead className="w-64">监控地址</TableHead>
                    <TableHead>检测间隔</TableHead>
                    <TableHead>请求方法</TableHead>
                    <TableHead>请求头</TableHead>
                    <TableHead>请求体</TableHead>
                    <TableHead className="w-24 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pagination.visibleItems.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell>{item.name}</TableCell>
                      <TableCell>
                        {monitorTypeLabels[item.monitoring?.type ?? "http"] ??
                          item.monitoring?.type}
                      </TableCell>
                      <TableCell>
                        {item.monitoring?.url || (item.url_configured ? "已配置" : "使用时填写")}
                      </TableCell>
                      <TableCell>
                        {item.monitoring ? `${item.monitoring.interval} 秒` : "使用时设置"}
                      </TableCell>
                      <TableCell>{item.method}</TableCell>
                      <TableCell>{item.headers_configured ? "已配置" : "未配置"}</TableCell>
                      <TableCell>{item.body_configured ? "已配置" : "未配置"}</TableCell>
                      <TableCell overflowTooltip={false}>
                        <div
                          role="group"
                          aria-label={`${item.name} 的操作`}
                          className="flex justify-end gap-1"
                        >
                          <TableActionButton
                            label="编辑"
                            disabled={disabled}
                            onClick={() => {
                              save.reset();
                              setEditor({ item });
                            }}
                          >
                            <Pencil aria-hidden="true" />
                          </TableActionButton>
                          <TableActionButton
                            label="删除"
                            tone="danger"
                            disabled={disabled}
                            onClick={() => setDeleting(item)}
                          >
                            <Trash2 aria-hidden="true" />
                          </TableActionButton>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {!items.length && (
                    <TableEmptyState columns={8}>
                      {search ? "没有匹配的模板" : "暂无功能模板"}
                    </TableEmptyState>
                  )}
                </TableBody>
              </Table>
              <DataTablePagination
                currentPage={pagination.currentPage}
                totalPages={pagination.totalPages}
                totalItems={items.length}
                pageSize={pagination.pageSize}
                onPageChange={pagination.setCurrentPage}
                onPageSizeChange={pagination.setPageSize}
              />
            </DataTablePanel>
          </>
        )}
      </div>
      {editor && (
        <TemplateDialog
          item={editor.item}
          pending={save.isPending}
          error={save.error}
          onClose={() => setEditor(null)}
          onSubmit={(value) => save.mutate(value)}
        />
      )}
      <ConfirmActionDialog
        open={!!deleting}
        title="删除功能模板"
        description={`删除「${deleting?.name ?? ""}」后无法再次套用；已保存的监控项不会改变。`}
        confirmLabel="删除模板"
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={() => {
          if (deleting) remove.mutate(deleting);
        }}
      />
    </PageLayout>
  );
}
