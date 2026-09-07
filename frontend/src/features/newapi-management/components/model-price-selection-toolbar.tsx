import type { ReactElement } from "react";
import { ListChecks, RefreshCw, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function ModelPriceSelectionToolbar(props: {
  selectedCount: number;
  pending: boolean;
  selectAllDisabled: boolean;
  onClear: () => void;
  onSelectAll: () => void;
  onSync: () => void;
}): ReactElement | null {
  if (props.selectedCount === 0) return null;
  const overLimit = props.selectedCount > 1000;

  return (
    <div
      role="toolbar"
      aria-label={`已选择 ${props.selectedCount} 个模型的批量操作`}
      aria-describedby="model-price-bulk-actions-description"
      tabIndex={-1}
      onKeyDown={(event) => {
        if (event.key === "Escape" && !props.pending) {
          event.preventDefault();
          props.onClear();
        }
      }}
      className="fixed bottom-6 left-1/2 z-50 max-w-[calc(100%-2rem)] -translate-x-1/2 rounded-xl transition-all duration-300 ease-out hover:scale-105 focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
    >
      <div className="flex items-center gap-x-2 rounded-xl border bg-background/95 p-2 shadow-xl supports-[backdrop-filter]:bg-background/60 supports-[backdrop-filter]:backdrop-blur-lg">
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
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
          id="model-price-bulk-actions-description"
          className="flex items-center gap-x-1 whitespace-nowrap text-sm"
          aria-live="polite"
        >
          <Badge
            variant="default"
            className="min-w-8 rounded-lg"
            aria-label={`${props.selectedCount} 个已选择模型`}
          >
            {props.selectedCount}
          </Badge>
          <span className="hidden sm:inline">模型</span>
          <span>已选择</span>
        </div>
        <div className="h-5 border-l" aria-hidden="true" />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="size-8"
                aria-label="全选筛选结果"
                disabled={props.pending || props.selectAllDisabled}
                onClick={props.onSelectAll}
              />
            }
          >
            <ListChecks aria-hidden="true" />
          </TooltipTrigger>
          <TooltipContent>全选筛选结果</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="size-8"
                aria-label={`批量同步（${props.selectedCount}）`}
                aria-describedby={overLimit ? "model-price-bulk-limit" : undefined}
                disabled={props.pending || overLimit}
                onClick={props.onSync}
              />
            }
          >
            <RefreshCw aria-hidden="true" />
          </TooltipTrigger>
          <TooltipContent>同步已选择模型价格</TooltipContent>
        </Tooltip>
      </div>
      {overLimit ? (
        <p
          id="model-price-bulk-limit"
          role="status"
          className="mt-2 rounded-lg border bg-background px-3 py-2 text-center text-xs text-destructive shadow-xl"
        >
          每批最多同步 1000 个模型
        </p>
      ) : null}
    </div>
  );
}
