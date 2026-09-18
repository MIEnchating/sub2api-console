import type { NewAPIChannel } from "@/api";
import { ListMinus, ListPlus } from "lucide-react";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { Badge } from "@/components/ui/badge";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ChannelModelAction } from "../lib/channel-model-change";
import { channelStatusLabels } from "../constants";

export function ChannelTable(props: {
  items: NewAPIChannel[];
  emptyMessage: string;
  selected: ReadonlyMap<string, NewAPIChannel>;
  disabled: boolean;
  onSelect: (channel: NewAPIChannel, checked: boolean) => void;
  onSelectVisible: (checked: boolean) => void;
  onEdit: (channels: NewAPIChannel[], action: ChannelModelAction) => void;
}) {
  const count = props.items.filter((channel) => props.selected.has(channel.id)).length;
  return (
    <Table
      aria-label="现有渠道"
      actionColumn
      containerClassName="min-h-0 flex-1 overflow-auto"
      className="min-w-[800px]"
    >
      <TableHeader>
        <TableRow>
          <TableHead className="w-11">
            <Checkbox
              aria-label="全选当前筛选渠道"
              checked={props.items.length > 0 && count === props.items.length}
              indeterminate={count > 0 && count < props.items.length}
              disabled={
                props.disabled ||
                props.items.length === 0 ||
                props.selected.size + props.items.length - count > 50
              }
              onCheckedChange={props.onSelectVisible}
            />
          </TableHead>
          <TableHead className="w-48">渠道</TableHead>
          <TableHead className="w-28">状态</TableHead>
          <TableHead className="w-36">分组</TableHead>
          <TableHead>已上架模型</TableHead>
          <TableHead className="w-24">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.items.length === 0 && (
          <TableEmptyState columns={6}>{props.emptyMessage}</TableEmptyState>
        )}
        {props.items.map((channel) => (
          <TableRow
            key={channel.id}
            aria-label={`渠道 ${channel.name}（${channel.id}）`}
            aria-selected={props.selected.has(channel.id)}
            data-state={props.selected.has(channel.id) ? "selected" : undefined}
          >
            <TableCell overflowTooltip={false}>
              <Checkbox
                aria-label={`选择渠道 ${channel.name}（${channel.id}）`}
                checked={props.selected.has(channel.id)}
                disabled={
                  props.disabled || (!props.selected.has(channel.id) && props.selected.size >= 50)
                }
                onCheckedChange={(checked) => props.onSelect(channel, checked)}
              />
            </TableCell>
            <TableCell overflowTooltip={false}>
              <Tooltip>
                <TooltipTrigger render={<p tabIndex={0} className="truncate font-medium" />}>
                  {channel.name}
                </TooltipTrigger>
                <TooltipContent role="tooltip">{channel.name}</TooltipContent>
              </Tooltip>
              <p className="text-muted-foreground text-xs">ID {channel.id}</p>
            </TableCell>
            <TableCell>
              <Badge variant={channel.status === 1 ? "secondary" : "outline"}>
                {channelStatusLabels[channel.status] ?? "状态未识别"}
              </Badge>
            </TableCell>
            <TableCell title={channel.groups.join("、")}>
              {channel.groups.join("、") || "未分组"}
            </TableCell>
            <TableCell overflowTooltip={false}>
              <p className="text-sm">{channel.models.length} 个模型</p>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <p tabIndex={0} className="truncate font-mono text-xs text-muted-foreground" />
                  }
                >
                  {channel.models.join("、") || "暂无模型"}
                </TooltipTrigger>
                <TooltipContent role="tooltip">
                  {channel.models.join("、") || "暂无模型"}
                </TooltipContent>
              </Tooltip>
            </TableCell>
            <TableCell overflowTooltip={false}>
              <div className="flex items-center gap-2">
                <TableActionButton
                  label="上架模型"
                  tone="primary"
                  disabled={props.disabled}
                  onClick={() => props.onEdit([channel], "add")}
                >
                  <ListPlus aria-hidden="true" />
                </TableActionButton>
                <TableActionButton
                  label="下架模型"
                  tone="danger"
                  disabled={props.disabled || channel.models.length === 0}
                  onClick={() => props.onEdit([channel], "remove")}
                >
                  <ListMinus aria-hidden="true" />
                </TableActionButton>
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
