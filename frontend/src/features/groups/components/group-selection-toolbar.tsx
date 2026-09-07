import { X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { groupBatchActionOrder, groupBatchActions, type GroupBatchAction } from "../constants";

export function GroupSelectionToolbar(props: {
  selectedCount: number;
  pending: boolean;
  disabled: boolean;
  onClear: () => void;
  onAction: (action: GroupBatchAction) => void;
}) {
  if (props.selectedCount === 0) return null;
  return (
    <div
      role="toolbar"
      aria-label={`已选择 ${props.selectedCount} 个分组的批量操作`}
      aria-describedby="group-bulk-actions-description"
      tabIndex={-1}
      onKeyDown={(event) => {
        if (event.key === "Escape" && !props.pending) {
          event.preventDefault();
          props.onClear();
        }
      }}
      className="fixed bottom-6 left-1/2 z-50 max-w-[calc(100vw-2rem)] -translate-x-1/2 rounded-xl transition-all duration-300 ease-out focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
    >
      <div className="flex items-center gap-x-2 rounded-xl border bg-background/95 p-2 shadow-xl supports-[backdrop-filter]:bg-background/60 supports-[backdrop-filter]:backdrop-blur-lg">
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="outline"
                size="icon"
                className="size-6"
                aria-label="清空选择"
                disabled={props.pending}
                onClick={props.onClear}
              />
            }
          >
            <X aria-hidden="true" />
          </TooltipTrigger>
          <TooltipContent>清空选择（Esc）</TooltipContent>
        </Tooltip>
        <div className="h-5 border-l" aria-hidden="true" />
        <div
          id="group-bulk-actions-description"
          className="flex items-center gap-x-1 whitespace-nowrap text-sm"
          aria-live="polite"
        >
          <Badge variant="default" className="min-w-8 rounded-lg">
            {props.selectedCount}
          </Badge>
          <span className="hidden sm:inline">分组</span>
          <span>已选择</span>
        </div>
        <div className="h-5 border-l" aria-hidden="true" />
        {groupBatchActionOrder.map((action) => {
          const meta = groupBatchActions[action];
          const Icon = meta.icon;
          return (
            <Tooltip key={action}>
              <TooltipTrigger
                render={
                  <Button
                    variant={action === "exclude" ? "destructive" : "outline"}
                    size="icon"
                    className="size-8"
                    aria-label={meta.label}
                    disabled={props.pending || props.disabled}
                    onClick={() => props.onAction(action)}
                  />
                }
              >
                <Icon aria-hidden="true" />
              </TooltipTrigger>
              <TooltipContent>{meta.label}</TooltipContent>
            </Tooltip>
          );
        })}
      </div>
    </div>
  );
}
