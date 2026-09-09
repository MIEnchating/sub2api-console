import type { ReactElement } from "react";
import { Radio, RefreshCw, WifiOff } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { liveResultsPresentation } from "../lib/live-results-status";
import type { LiveResultsStatus } from "../lib/live-results-status";

export function AccountLiveStatus(props: { status: LiveResultsStatus }): ReactElement | null {
  if (props.status === "idle") return null;
  const presentation = liveResultsPresentation[props.status];
  let Icon = Radio;
  if (props.status === "connecting") Icon = RefreshCw;
  if (["reconnecting", "retrying", "unsupported"].includes(props.status)) Icon = WifiOff;
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            role="status"
            aria-label={presentation.label}
            aria-description={presentation.detail}
            tabIndex={0}
            className={cn(
              "inline-flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring",
              props.status === "connected" && "text-success",
              ["reconnecting", "retrying", "unsupported"].includes(props.status) && "text-warning",
            )}
          />
        }
      >
        <Icon aria-hidden="true" className="size-3.5" />
      </TooltipTrigger>
      <TooltipContent role="tooltip" className="max-w-64 text-xs font-normal">
        <div className="grid gap-1">
          <span className="font-medium">{presentation.label}</span>
          <span>{presentation.detail}</span>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
