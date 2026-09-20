import { useCallback, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Folder, ListChecks, ListMinus, ListPlus, Plus } from "lucide-react";
import { toast } from "sonner";
import { api, type NewAPIChannel, type NewAPIChannelGroups } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { QueryErrorToast } from "@/components/query-error-toast";
import { RefreshButton } from "@/components/refresh-button";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { SearchField } from "@/components/data-table/search-field";
import { SelectionToolbar } from "@/components/data-table/selection-toolbar";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { PageActions } from "@/components/page-actions";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { notifyOperationError } from "@/lib/operation-feedback";
import { useChannelMaintenance } from "../hooks/use-channel-maintenance";
import { ChannelMaintenanceDialog } from "./channel-maintenance-dialog";
import { ChannelTable } from "./channel-table";
import { ChannelTaskDialog } from "./channel-task-dialog";
import { ChannelGroupDialog } from "./channel-group-dialog";

export function ExistingChannels(props: { platformId: string; onCreate?: () => void }) {
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState(new Map<string, NewAPIChannel>());
  const [groupFilter, setGroupFilter] = useState("all");
  const [groupOpen, setGroupOpen] = useState(false);
  const [selectingGroup, setSelectingGroup] = useState(false);
  const queryClient = useQueryClient();
  const channels = useQuery({
    queryKey: ["newapi-channels", props.platformId, page, pageSize, groupFilter],
    queryFn: () =>
      api.newAPIChannels(
        props.platformId,
        page,
        pageSize,
        groupFilter === "all" ? "" : groupFilter,
      ),
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[4] === groupFilter ? keepPreviousData(previous) : undefined,
  });
  const channelGroups = useQuery({
    queryKey: ["newapi-channel-groups", props.platformId],
    queryFn: () => api.newAPIChannelGroups(props.platformId),
  });
  const saveGroups = useMutation({
    mutationFn: (payload: NewAPIChannelGroups) =>
      api.saveNewAPIChannelGroups(props.platformId, payload),
    onSuccess: (next) => {
      queryClient.setQueryData(["newapi-channel-groups", props.platformId], next);
      if (!next.groups.some((group) => group.id === groupFilter)) setGroupFilter("all");
      setPage(0);
      void queryClient.invalidateQueries({ queryKey: ["newapi-channels", props.platformId] });
      toast.success("渠道分组已保存");
    },
    onError: (error) => {
      notifyOperationError(error, "渠道分组保存失败");
      void channelGroups.refetch();
    },
  });
  const onCompleted = useCallback(() => setSelected(new Map()), []);
  const maintenance = useChannelMaintenance(props.platformId, onCompleted);
  const items = channels.data?.items ?? [];
  const query = search.trim().toLowerCase();
  const groups = channelGroups.data?.groups ?? [];
  const group = groups.find((item) => item.id === groupFilter);
  const visible = items.filter((channel) =>
    [channel.id, channel.name, ...channel.groups, ...channel.models].some((value) =>
      value.toLowerCase().includes(query),
    ),
  );
  const disabled =
    maintenance.change.isPending ||
    channels.isFetching ||
    maintenance.batchRunning ||
    selectingGroup;
  const total = channels.data?.total ?? 0;
  const totalItems =
    total === -1 && items.length < pageSize ? page * pageSize + items.length : total;
  const totalPages = totalItems === -1 ? page + 2 : Math.max(1, Math.ceil(totalItems / pageSize));
  function select(channel: NewAPIChannel, checked: boolean): void {
    setSelected((current) => {
      const next = new Map(current);
      if (checked && next.size < 50) next.set(channel.id, channel);
      else if (!checked) next.delete(channel.id);
      return next;
    });
  }
  function selectVisible(checked: boolean): void {
    setSelected((current) => {
      const next = new Map(current);
      for (const channel of visible) {
        if (checked && next.size < 50) next.set(channel.id, channel);
        else if (!checked) next.delete(channel.id);
      }
      return next;
    });
  }
  async function selectGroup(): Promise<void> {
    if (!group || group.channel_ids.length === 0 || group.channel_ids.length > 50) return;
    setSelectingGroup(true);
    try {
      const result = await queryClient.fetchQuery({
        queryKey: ["newapi-channels", props.platformId, 0, 100, group.id],
        queryFn: () => api.newAPIChannels(props.platformId, 0, 100, group.id),
        staleTime: 0,
      });
      if (result.total > 50 || result.items.length > 50)
        throw new Error("分组已超过 50 个渠道，请分页选择");
      setSelected(new Map(result.items.map((channel) => [channel.id, channel])));
    } catch (error) {
      notifyOperationError(error, "选择渠道分组失败");
    } finally {
      setSelectingGroup(false);
    }
  }
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      <TableFilterToolbar>
        <SearchField value={search} onChange={setSearch} placeholder="搜索本页渠道、分组或模型" />
        <Select
          value={groupFilter}
          disabled={selectingGroup}
          onValueChange={(value) => {
            setGroupFilter(value ?? "all");
            setPage(0);
            setSearch("");
          }}
        >
          <SelectTrigger aria-label="按渠道分组筛选" className="w-full sm:w-44">
            <SelectValue>
              {group ? `${group.name}（${group.channel_ids.length}）` : "全部渠道分组"}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部渠道分组</SelectItem>
            {groups.map((item) => (
              <SelectItem key={item.id} value={item.id}>
                {item.name}（{item.channel_ids.length}）
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {group && (
          <Button
            variant="outline"
            disabled={disabled || group.channel_ids.length === 0 || group.channel_ids.length > 50}
            onClick={() => void selectGroup()}
          >
            <ListChecks aria-hidden="true" />
            选择整组
          </Button>
        )}
        <PageActions className="sm:ml-auto">
          {maintenance.task && (
            <Button variant="outline" onClick={() => maintenance.setTaskOpen(true)}>
              查看批量任务
            </Button>
          )}
          <RefreshButton
            pending={channels.isFetching || channelGroups.isFetching}
            ariaLabel="刷新渠道列表"
            onClick={() => {
              void channels.refetch();
              void channelGroups.refetch();
            }}
          />
          {props.onCreate && (
            <Button onClick={props.onCreate}>
              <Plus aria-hidden="true" />
              新增渠道
            </Button>
          )}
          <Button variant="outline" onClick={() => setGroupOpen(true)}>
            <Folder aria-hidden="true" />
            渠道分组
          </Button>
          {channelGroups.isError && (
            <Button
              variant="outline"
              disabled={channelGroups.isFetching}
              onClick={() => void channelGroups.refetch()}
            >
              重试渠道分组
            </Button>
          )}
        </PageActions>
      </TableFilterToolbar>
      <DataTablePanel className="flex min-h-0 flex-1 flex-col">
        {channels.isPending && <PageLoadingSkeleton label="正在读取现有渠道" fill />}
        {!channels.data && channels.isError && (
          <ContentRetry pending={channels.isFetching} onRetry={() => void channels.refetch()} />
        )}
        {channels.data && (
          <ChannelTable
            items={visible}
            emptyMessage={search || group ? "没有匹配的渠道" : "暂无渠道"}
            selected={selected}
            disabled={disabled}
            onSelect={select}
            onSelectVisible={selectVisible}
            onEdit={(channels, action) => maintenance.setEditing({ channels, action })}
          />
        )}
        {channels.data && (
          <DataTablePagination
            currentPage={page + 1}
            totalPages={Math.min(1000, totalPages)}
            totalItems={totalItems}
            pageSize={pageSize}
            disabled={channels.isFetching || !channels.data}
            onPageChange={(next) => {
              setPage(next - 1);
              setSearch("");
            }}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setPage(0);
              setSearch("");
            }}
          />
        )}
      </DataTablePanel>
      <SelectionToolbar
        selectedCount={selected.size}
        entityLabel="渠道"
        pending={disabled}
        onClear={() => setSelected(new Map())}
        message={
          <p className="text-muted-foreground mt-2 text-center text-xs">
            跨页保留选择，每次最多 50 个渠道
          </p>
        }
      >
        <TableActionButton label="管理渠道分组" tone="primary" onClick={() => setGroupOpen(true)}>
          <Folder aria-hidden="true" />
        </TableActionButton>
        <TableActionButton
          label="批量上架模型"
          tone="primary"
          disabled={disabled}
          onClick={() =>
            maintenance.setEditing({ channels: [...selected.values()], action: "add" })
          }
        >
          <ListPlus aria-hidden="true" />
        </TableActionButton>
        <TableActionButton
          label="批量下架模型"
          tone="danger"
          disabled={
            disabled || [...selected.values()].every((channel) => channel.models.length === 0)
          }
          onClick={() =>
            maintenance.setEditing({ channels: [...selected.values()], action: "remove" })
          }
        >
          <ListMinus aria-hidden="true" />
        </TableActionButton>
      </SelectionToolbar>
      {channelGroups.error && (
        <QueryErrorToast error={channelGroups.error} fallback="渠道分组读取失败" />
      )}
      {maintenance.editing && (
        <ChannelMaintenanceDialog
          channels={maintenance.editing.channels}
          action={maintenance.editing.action}
          pending={maintenance.change.isPending}
          onClose={() => maintenance.setEditing(null)}
          onSubmit={async (input) => {
            await maintenance.change.mutateAsync(input);
          }}
        />
      )}
      {maintenance.currentTask && (
        <ChannelTaskDialog
          task={maintenance.currentTask}
          open={maintenance.taskOpen}
          error={maintenance.taskQuery.isError}
          retrying={maintenance.taskQuery.isFetching}
          onRetry={() => void maintenance.taskQuery.refetch()}
          onOpenChange={maintenance.setTaskOpen}
        />
      )}
      {groupOpen && channelGroups.data && (
        <ChannelGroupDialog
          platformId={props.platformId}
          channels={items}
          groups={groups}
          selected={[...selected.values()]}
          version={channelGroups.data.version}
          pending={saveGroups.isPending}
          onClose={() => setGroupOpen(false)}
          onSave={async (next, version) => {
            await saveGroups.mutateAsync({ groups: next, version });
          }}
        />
      )}
      {groupOpen && !channelGroups.data && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setGroupOpen(false);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>渠道分组</DialogTitle>
            </DialogHeader>
            <DialogBody>
              {channelGroups.isPending && <ContentLoading label="正在读取渠道分组" />}
              {channelGroups.isError && (
                <ContentRetry
                  pending={channelGroups.isFetching}
                  onRetry={() => void channelGroups.refetch()}
                />
              )}
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" onClick={() => setGroupOpen(false)}>
                取消
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
