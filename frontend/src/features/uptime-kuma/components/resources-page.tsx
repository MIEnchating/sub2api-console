import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, ExternalLink, Pause, Pencil, Play, Send, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type KumaResource, type KumaResourceKind, type KumaResourceWrite } from "@/api";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { PageActions } from "@/components/page-actions";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { SearchField } from "@/components/data-table/search-field";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { notifyOperationError } from "@/lib/operation-feedback";
import {
  kumaConfigKey,
  kumaQueryKey,
  kumaResourcesKey,
  resourceTitles,
  resourceActionLabels,
  notificationTypes,
  maintenanceStrategies,
} from "../constants";
import { useTaskCompletion } from "../hooks/use-task-completion";
import type { ResourceValues } from "../lib/resource-schemas";
import { ResourceDialog } from "./resource-dialog";

type ConfirmedAction = "delete" | "test" | "pause" | "resume";
function confirmationDescription(
  value: { item: KumaResource; action: ConfirmedAction } | null,
): string {
  if (!value) return "";
  let consequence = "会改变该维护计划的生效状态。";
  if (value.action === "test") consequence = "会向该渠道发送一条真实测试通知。";
  if (value.action === "delete") consequence = "删除无法撤销，并会移除关联配置。";
  return `将对「${value.item.name}」（ID ${value.item.id}）执行${resourceActionLabels[value.action]}。${consequence}`;
}
export function KumaResourcesPage(props: { kind: KumaResourceKind }) {
  const client = useQueryClient();
  const task = useTaskCompletion();
  const [search, setSearch] = useState("");
  const [editor, setEditor] = useState<{ item?: KumaResource } | null>(null);
  const [confirmation, setConfirmation] = useState<{
    item: KumaResource;
    action: ConfirmedAction;
  } | null>(null);
  const config = useQuery({ queryKey: kumaConfigKey, queryFn: api.kumaConfig, retry: false });
  const query = useQuery({
    queryKey: [...kumaResourcesKey, props.kind, config.data?.revision],
    queryFn: () => api.kumaResources(props.kind),
    enabled: !!config.data?.management_configured,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const refresh = (): Promise<void> => client.invalidateQueries({ queryKey: kumaQueryKey });
  const detail = useMutation({
    mutationFn: (id: number) => api.kumaResource(props.kind, id),
    onSuccess: (item) => {
      save.reset();
      setEditor({ item });
    },
    onError: (error) => notifyOperationError(error, "编辑数据读取失败"),
  });
  const save = useMutation({
    mutationFn: (value: { id: number; input: KumaResourceWrite }) =>
      api.writeKumaResource(props.kind, value.id, value.input).then(task.wait),
    onSuccess: async () => {
      setEditor(null);
      setConfirmation(null);
      toast.success("Uptime Kuma 操作已完成");
      await refresh();
    },
    onError: async (error) => {
      if (!editor) notifyOperationError(error, "管理操作失败");
      await refresh();
    },
  });
  const pending = save.isPending || detail.isPending;
  const disabled = pending || query.isFetching || query.isError || !query.data || config.isError;
  const items = (query.data?.items ?? []).filter((item) =>
    `${item.name} ${item.type}`.toLowerCase().includes(search.trim().toLowerCase()),
  );
  const pagination = useClientPagination(items);
  const submit = (values: ResourceValues): void => {
    if (!config.data) return;
    const item = editor?.item;
    const input: KumaResourceWrite = {
      config_revision: config.data.revision,
      revision: item?.revision ?? "",
      association_revision: item?.association_revision ?? "",
      action: item ? "edit" : "create",
    };
    if (props.kind === "notifications") input.notification = values.notification;
    if (props.kind === "maintenance") input.maintenance = values.maintenance;
    if (props.kind === "status-pages") input.status_page = values.status_page;
    save.mutate({ id: item?.id ?? 0, input });
  };
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="Uptime Kuma"
        title={resourceTitles[props.kind]}
        description=""
        action={
          <PageActions>
            <RefreshButton
              pending={config.isFetching || query.isFetching}
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
              新增{resourceTitles[props.kind]}
            </Button>
          </PageActions>
        }
      />
      {(config.error || query.error) && (
        <QueryErrorToast error={config.error ?? query.error} fallback="管理数据读取失败" />
      )}
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        {config.isPending && <PageLoadingSkeleton label="正在读取接入配置…" variant="table" />}
        {config.data && !config.data.management_configured && (
          <p className="text-muted-foreground text-sm">
            请在侧栏「接入配置」中验证管理账号后使用此功能。
          </p>
        )}
        {config.data?.management_configured && (
          <>
            {query.isPending && <PageLoadingSkeleton label="正在读取管理数据…" variant="table" />}
            {detail.isPending && <PageLoadingSkeleton label="正在读取编辑配置…" variant="form" />}
            {!query.isPending && (
              <>
                <TableFilterToolbar>
                  <SearchField
                    value={search}
                    onChange={(v) => {
                      setSearch(v);
                      pagination.setCurrentPage(1);
                    }}
                    placeholder="搜索名称或类型"
                  />
                </TableFilterToolbar>
                <DataTablePanel className="flex-1">
                  <Table
                    aria-label={resourceTitles[props.kind]}
                    className="min-w-[42rem]"
                    containerClassName="min-h-0 flex-1 overflow-auto"
                  >
                    <TableHeader>
                      <TableRow>
                        <TableHead>名称</TableHead>
                        <TableHead>类型 / 路径</TableHead>
                        <TableHead className="w-28">状态</TableHead>
                        <TableHead className="w-36 text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {pagination.visibleItems.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell>{item.name}</TableCell>
                          <TableCell>
                            {(props.kind === "notifications"
                              ? notificationTypes[item.type]
                              : maintenanceStrategies[item.type]) ?? item.type}
                          </TableCell>
                          <TableCell>
                            <StatusBadge
                              label={item.active ? "已配置" : "已暂停"}
                              variant={item.active ? "success" : "neutral"}
                            />
                          </TableCell>
                          <TableCell overflowTooltip={false}>
                            <div
                              className="flex items-center justify-end gap-1"
                              role="group"
                              aria-label={`${item.name} 的操作`}
                            >
                              <TableActionButton
                                label="编辑"
                                disabled={
                                  disabled ||
                                  (props.kind === "notifications" && !notificationTypes[item.type])
                                }
                                onClick={() => detail.mutate(item.id)}
                              >
                                <Pencil aria-hidden="true" />
                              </TableActionButton>
                              {props.kind === "notifications" && (
                                <TableActionButton
                                  label="发送测试通知"
                                  disabled={disabled}
                                  onClick={() => setConfirmation({ item, action: "test" })}
                                >
                                  <Send aria-hidden="true" />
                                </TableActionButton>
                              )}
                              {props.kind === "maintenance" && (
                                <TableActionButton
                                  label={item.active ? "暂停" : "恢复"}
                                  disabled={disabled}
                                  onClick={() =>
                                    setConfirmation({
                                      item,
                                      action: item.active ? "pause" : "resume",
                                    })
                                  }
                                >
                                  {item.active ? (
                                    <Pause aria-hidden="true" />
                                  ) : (
                                    <Play aria-hidden="true" />
                                  )}
                                </TableActionButton>
                              )}
                              {props.kind === "status-pages" && (
                                <Button
                                  variant="outline"
                                  size="icon"
                                  role="link"
                                  aria-label="打开状态页"
                                  render={
                                    <a
                                      href={`${config.data?.base_url}/status/${encodeURIComponent(item.type)}`}
                                      target="_blank"
                                      rel="noreferrer"
                                    />
                                  }
                                >
                                  <ExternalLink aria-hidden="true" />
                                </Button>
                              )}
                              <TableActionButton
                                label="删除"
                                tone="danger"
                                disabled={disabled}
                                onClick={() => setConfirmation({ item, action: "delete" })}
                              >
                                <Trash2 aria-hidden="true" />
                              </TableActionButton>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                      {!pagination.visibleItems.length && (
                        <TableEmptyState columns={4}>
                          {search ? "没有匹配的记录" : "暂无记录"}
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
          </>
        )}
      </div>
      {editor && query.data && (
        <ResourceDialog
          key={`${props.kind}:${editor.item?.id ?? 0}`}
          kind={props.kind}
          item={editor.item}
          options={query.data}
          pending={save.isPending}
          task={task.task}
          error={save.error}
          onClose={() => setEditor(null)}
          onSubmit={submit}
        />
      )}
      <ConfirmActionDialog
        open={!!confirmation}
        title={confirmation ? resourceActionLabels[confirmation.action] : "确认操作"}
        description={confirmationDescription(confirmation)}
        confirmLabel={confirmation ? resourceActionLabels[confirmation.action] : "确认"}
        pending={save.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation && config.data)
            save.mutate({
              id: confirmation.item.id,
              input: {
                config_revision: config.data.revision,
                revision: confirmation.item.revision,
                association_revision: "",
                action: confirmation.action,
              },
            });
        }}
      />
    </PageLayout>
  );
}
