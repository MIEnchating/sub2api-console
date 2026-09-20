import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type NewAPIChannel } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { DataTablePagination } from "@/components/data-table/pagination";
import { SearchField } from "@/components/data-table/search-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

export function ChannelGroupMemberDialog(props: {
  platformId: string;
  groupName: string;
  memberIDs: string[];
  onClose: () => void;
  onAdd: (channels: NewAPIChannel[]) => void;
}) {
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState(new Map<string, NewAPIChannel>());
  const channels = useQuery({
    queryKey: ["newapi-channels", props.platformId, page, pageSize, "all"],
    queryFn: () => api.newAPIChannels(props.platformId, page, pageSize),
  });
  const items = channels.data?.items ?? [];
  const query = search.trim().toLocaleLowerCase();
  const visible = items.filter((channel) =>
    `${channel.name} ${channel.id}`.toLocaleLowerCase().includes(query),
  );
  const total = channels.data?.total ?? 0;
  const totalItems =
    total === -1 && items.length < pageSize ? page * pageSize + items.length : total;
  const totalPages = totalItems === -1 ? page + 2 : Math.max(1, Math.ceil(totalItems / pageSize));
  const full = props.memberIDs.length + selected.size >= 1000;

  function toggle(channel: NewAPIChannel, checked: boolean): void {
    setSelected((current) => {
      const next = new Map(current);
      if (checked && props.memberIDs.length + next.size < 1000) next.set(channel.id, channel);
      else if (!checked) next.delete(channel.id);
      return next;
    });
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="progress" height="large" className="h-[min(36rem,calc(100svh-2rem))]">
        <DialogHeader>
          <DialogTitle>添加渠道到分组</DialogTitle>
          <DialogDescription>
            选择要加入「{props.groupName || "未命名分组"}」的渠道，支持跨页选择。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="flex flex-col gap-3 overflow-hidden pr-0">
          <SearchField value={search} onChange={setSearch} placeholder="搜索本页渠道名称或 ID" />
          <div
            role="region"
            aria-label="可添加的渠道"
            className="min-h-0 flex-1 overflow-y-auto rounded-lg border"
          >
            {!channels.data && channels.isPending && (
              <ContentLoading label="正在读取可添加的渠道" />
            )}
            {!channels.data && channels.isError && (
              <ContentRetry pending={channels.isFetching} onRetry={() => void channels.refetch()} />
            )}
            {channels.data && visible.length === 0 && (
              <p className="text-muted-foreground px-4 py-10 text-center text-sm">
                {query ? "本页没有匹配的渠道" : "暂无可添加的渠道"}
              </p>
            )}
            {visible.map((channel) => {
              const member = props.memberIDs.includes(channel.id);
              const checked = member || selected.has(channel.id);
              return (
                <label
                  key={channel.id}
                  className={cn(
                    "flex min-w-0 items-center gap-3 border-b px-3 py-3 last:border-b-0",
                    checked ? "bg-primary/5" : "hover:bg-muted/50",
                  )}
                >
                  <Checkbox
                    aria-labelledby=""
                    aria-label={`选择渠道 ${channel.name}（${channel.id}）`}
                    checked={checked}
                    disabled={member || channels.isFetching || (full && !checked)}
                    onCheckedChange={(value) => toggle(channel, value)}
                  />
                  <span className="min-w-0 flex-1">
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <span tabIndex={0} className="block truncate text-sm font-medium" />
                        }
                      >
                        {channel.name}
                      </TooltipTrigger>
                      <TooltipContent role="tooltip">{channel.name}</TooltipContent>
                    </Tooltip>
                    <span className="text-muted-foreground block text-xs">#{channel.id}</span>
                  </span>
                  {member && (
                    <span className="text-muted-foreground shrink-0 text-xs">已在分组</span>
                  )}
                </label>
              );
            })}
          </div>
          {channels.isError && channels.data && (
            <ContentRetry pending={channels.isFetching} onRetry={() => void channels.refetch()} />
          )}
          {channels.data && (
            <DataTablePagination
              currentPage={page + 1}
              totalPages={Math.min(1000, totalPages)}
              totalItems={totalItems}
              pageSize={pageSize}
              disabled={channels.isFetching}
              onPageChange={(value) => {
                setPage(value - 1);
                setSearch("");
              }}
              onPageSizeChange={(value) => {
                setPageSize(value);
                setPage(0);
                setSearch("");
              }}
            />
          )}
        </DialogBody>
        <DialogFooter className="items-center sm:justify-between">
          <p className="text-muted-foreground text-xs">
            已选 {selected.size} 个 · 每组最多 1000 个渠道
          </p>
          <div className="flex w-full justify-end gap-2 sm:w-auto">
            <Button type="button" variant="outline" onClick={props.onClose}>
              取消
            </Button>
            <Button
              type="button"
              disabled={selected.size === 0 || channels.isFetching || channels.isError}
              onClick={() => props.onAdd([...selected.values()])}
            >
              添加到分组（{selected.size}）
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
