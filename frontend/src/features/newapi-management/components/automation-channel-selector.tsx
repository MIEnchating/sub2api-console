import { useId, type ReactElement } from "react";
import { Trash2 } from "lucide-react";
import type { AutomationChannelPageControls } from "../hooks/use-automation-channel-catalog";
import { DataTablePagination } from "@/components/data-table/pagination";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { RefreshButton } from "@/components/refresh-button";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  automationChannelLimit,
  type AutomationChannelSelection,
} from "../lib/automation-channel-selection";

export function AutomationChannelSelector(props: {
  selection: AutomationChannelSelection;
  pageControls?: AutomationChannelPageControls;
  disabled: boolean;
  pending?: boolean;
  onRefresh?: () => void;
  onToggle: (id: string) => void;
}): ReactElement {
  const helpID = useId();
  const full = props.selection.selected.size >= automationChannelLimit;
  const pagination = props.pageControls?.pagination;
  const showPagination =
    pagination &&
    (pagination.totalItems < 0 ||
      pagination.totalItems > pagination.pageSize ||
      pagination.currentPage > 1);
  return (
    <div
      role="group"
      className="grid min-w-0 content-start gap-2"
      aria-label="目标渠道"
      aria-invalid={props.selection.targets === null || undefined}
      aria-describedby={helpID}
      aria-disabled={props.disabled || props.pending || undefined}
    >
      <div className="text-sm font-medium">
        目标渠道 <span className="text-muted-foreground font-normal">（可选）</span>
      </div>
      <div className="grid h-40 content-start gap-1 overflow-auto overscroll-contain rounded-md border bg-muted/20 p-1">
        {props.selection.unavailableIDs.map((id) => (
          <div key={id} className="flex min-h-9 items-center gap-2 rounded px-2 text-sm">
            <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">渠道 {id}（未读取到）</span>
            <Button
              size="icon"
              variant="ghost"
              aria-label={`移除未读取到的渠道 ${id}`}
              disabled={props.disabled || props.pending}
              onClick={() => props.onToggle(id)}
            >
              <Trash2 aria-hidden="true" />
            </Button>
          </div>
        ))}
        {props.selection.options.map((channel) => {
          const checked = props.selection.selected.has(channel.id);
          return (
            <label
              key={channel.id}
              className="flex min-h-9 items-center gap-2 rounded px-2 text-sm hover:bg-background has-[[data-checked]]:bg-primary/10"
            >
              <Checkbox
                checked={checked}
                disabled={props.disabled || props.pending || (full && !checked)}
                onCheckedChange={() => props.onToggle(channel.id)}
              />
              <span className="min-w-0 truncate">
                {channel.name}（{channel.id}）
              </span>
            </label>
          );
        })}
        {props.selection.options.length === 0 && props.selection.unavailableIDs.length === 0 && (
          <p className="text-muted-foreground p-3 text-sm">当前平台没有可用渠道</p>
        )}
      </div>
      {props.pageControls?.pending && <ContentLoading compact label="正在读取渠道分页" />}
      {props.pageControls?.failed && (
        <ContentRetry
          onRetry={props.pageControls.retry}
          pending={props.disabled || props.pending || props.pageControls.pending}
        />
      )}
      {showPagination && (
        <DataTablePagination {...pagination} disabled={props.disabled || props.pending} />
      )}
      <div className="flex items-start gap-2">
        <div id={helpID} className="text-muted-foreground min-w-0 flex-1 text-xs">
          {props.selection.unavailableIDs.length > 0 && (
            <p>部分已选渠道无法确认，请重新读取或手动移除后再保存、预览。</p>
          )}
          {props.pageControls && <p>已选渠道跨页保留，其余显示当前页。</p>}
          <p>最多选择 {automationChannelLimit} 个渠道；不选渠道时，仅刷新账号模型目录。</p>
        </div>
        {props.onRefresh && (
          <RefreshButton
            ariaLabel="重新读取渠道"
            pending={props.pending}
            disabled={props.disabled}
            onClick={props.onRefresh}
          />
        )}
      </div>
    </div>
  );
}
