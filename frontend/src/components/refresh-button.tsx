import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export type RefreshButtonProps = {
  pending?: boolean;
  disabled?: boolean;
  onClick: () => void;
  ariaLabel?: string;
  testId?: string;
  className?: string;
};

export function RefreshButton(props: RefreshButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          aria-label={props.ariaLabel ?? "刷新"}
          data-testid={props.testId}
          disabled={props.disabled || props.pending}
          onClick={props.onClick}
          className={props.className}
        >
          <RefreshCw className={props.pending ? "animate-spin" : undefined} aria-hidden="true" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>刷新</TooltipContent>
    </Tooltip>
  );
}
