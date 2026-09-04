import type { ReactNode } from "react";
import { CircleHelp } from "lucide-react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export const fieldHelpTooltipPosition = {
  side: "inline-end",
  align: "start",
} as const;

export const fieldHelpTooltipContentStyles =
  "pointer-events-auto block max-w-sm whitespace-normal break-words select-text leading-5";

export function FieldHelpTooltip(props: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Tooltip disableHoverablePopup={false}>
      <TooltipTrigger
        render={
          <button
            type="button"
            className={cn(
              "text-muted-foreground hover:text-foreground focus-visible:border-ring focus-visible:ring-ring/50 inline-flex size-5 shrink-0 items-center justify-center rounded-full border border-transparent transition-colors outline-none focus-visible:ring-3",
              props.className,
            )}
            aria-label={`${props.label}说明`}
          />
        }
      >
        <CircleHelp className="size-3.5" aria-hidden="true" />
      </TooltipTrigger>
      <TooltipContent
        className={fieldHelpTooltipContentStyles}
        side={fieldHelpTooltipPosition.side}
        align={fieldHelpTooltipPosition.align}
      >
        {props.children}
      </TooltipContent>
    </Tooltip>
  );
}

export function FieldLabel(props: {
  label: string;
  description?: ReactNode;
  htmlFor?: string;
  className?: string;
}) {
  const label = props.htmlFor ? (
    <label htmlFor={props.htmlFor}>{props.label}</label>
  ) : (
    <span>{props.label}</span>
  );

  return (
    <span
      className={cn("inline-flex min-w-0 items-center gap-1 font-medium", props.className)}
      data-slot="field-label"
    >
      {label}
      {props.description ? (
        <FieldHelpTooltip label={props.label}>{props.description}</FieldHelpTooltip>
      ) : null}
    </span>
  );
}
