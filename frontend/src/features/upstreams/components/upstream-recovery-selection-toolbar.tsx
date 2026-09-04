import { ShieldCheck, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function UpstreamRecoverySelectionToolbar(props: {
  selectedCount: number;
  pending: boolean;
  onClear: () => void;
  onRecover: () => void;
}) {
  if (props.selectedCount === 0) return null;
  return (
    <div
      role="toolbar"
      aria-label={`已选择 ${props.selectedCount} 个上游的批量操作`}
      aria-describedby="upstream-recovery-bulk-actions-description"
      tabIndex={-1}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          props.onClear();
        }
      }}
      className="fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-xl transition-all duration-300 ease-out hover:scale-105 focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
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
                onClick={props.onClear}
              />
            }
          >
            <X />
          </TooltipTrigger>
          <TooltipContent>清空选择（Esc）</TooltipContent>
        </Tooltip>
        <div className="h-5 border-l" aria-hidden="true" />
        <div
          id="upstream-recovery-bulk-actions-description"
          className="flex items-center gap-x-1 text-sm"
          aria-live="polite"
        >
          <Badge
            variant="default"
            className="min-w-8 rounded-lg"
            aria-label={`${props.selectedCount} 个已选择上游`}
          >
            {props.selectedCount}
          </Badge>
          <span className="hidden sm:inline">上游</span>
          <span>已选择</span>
        </div>
        <div className="h-5 border-l" aria-hidden="true" />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                size="icon"
                className="size-8"
                aria-label={`恢复已选择的 ${props.selectedCount} 个上游鉴权`}
                disabled={props.pending}
                onClick={props.onRecover}
              />
            }
          >
            <ShieldCheck />
          </TooltipTrigger>
          <TooltipContent>恢复已选择上游鉴权</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
