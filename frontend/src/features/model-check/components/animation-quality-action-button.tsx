import { useId, type ReactElement, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function AnimationQualityActionButton(props: {
  label: string;
  description?: string;
  disabled: boolean;
  busy: boolean;
  onClick: () => void;
  children: ReactNode;
}): ReactElement {
  const descriptionID = useId();
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={props.label}
            aria-describedby={descriptionID}
            disabled={props.disabled}
            aria-busy={props.busy}
            onClick={props.onClick}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent id={descriptionID} role="tooltip">
        {props.description ?? props.label}
      </TooltipContent>
    </Tooltip>
  );
}
