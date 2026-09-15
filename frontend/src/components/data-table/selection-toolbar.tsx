import { useId, type ReactElement, type ReactNode } from "react";
import { X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export type SelectionToolbarProps = {
  selectedCount: number;
  entityLabel: string;
  pending: boolean;
  onClear: () => void;
  children: ReactNode;
  message?: ReactNode;
};

export function SelectionToolbar(props: SelectionToolbarProps): ReactElement | null {
  const descriptionId = useId();
  if (props.selectedCount === 0) return null;

  return (
    <div
      role="toolbar"
      aria-label={`已选择 ${props.selectedCount} 个${props.entityLabel}的批量操作`}
      aria-describedby={descriptionId}
      aria-busy={props.pending}
      tabIndex={-1}
      onKeyDown={(event) => {
        if (event.key === "Escape" && !event.defaultPrevented && !props.pending) {
          event.preventDefault();
          props.onClear();
        }
      }}
      className="fixed bottom-[max(7rem,env(safe-area-inset-bottom))] left-1/2 z-40 max-h-[calc(100svh-8rem)] sm:bottom-20 sm:max-h-[calc(100svh-6rem)] w-max max-w-[calc(100%-2rem)] -translate-x-1/2 overflow-y-auto rounded-lg border bg-popover p-2 text-popover-foreground shadow-lg outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex min-w-0 flex-wrap items-center justify-center gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="清空选择"
                  disabled={props.pending}
                  onClick={props.onClear}
                />
              }
            >
              <X aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>清空选择</TooltipContent>
          </Tooltip>
          <div
            id={descriptionId}
            className="flex min-w-0 items-center gap-1 whitespace-nowrap text-sm"
            aria-live="polite"
          >
            <Badge
              variant="default"
              className="min-w-8 rounded-md tabular-nums"
              aria-label={`${props.selectedCount} 个已选择${props.entityLabel}`}
            >
              {props.selectedCount}
            </Badge>
            <span className="hidden sm:inline">{props.entityLabel}</span>
            <span>已选择</span>
          </div>
        </div>
        <div className="flex min-w-0 flex-wrap items-center justify-center gap-2">
          {props.children}
        </div>
      </div>
      {props.message}
    </div>
  );
}
