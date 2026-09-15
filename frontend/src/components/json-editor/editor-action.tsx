import type { ComponentProps, ReactElement } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function EditorAction(
  props: ComponentProps<typeof Button> & { label: string },
): ReactElement {
  const { label, ...buttonProps } = props;
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="text-muted-foreground"
            {...buttonProps}
            aria-label={label}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent>{props.label}</TooltipContent>
    </Tooltip>
  );
}
