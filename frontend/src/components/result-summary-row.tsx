import { ExternalLink } from "lucide-react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function ResultSummaryRow(props: { label: string; value: string; href?: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 px-3 py-2.5 text-sm">
      <span className="text-muted-foreground shrink-0 whitespace-nowrap">{props.label}</span>
      <Tooltip>
        <TooltipTrigger
          render={<strong className="min-w-0 truncate text-right font-medium whitespace-nowrap" />}
        >
          {props.href ? (
            <a
              href={props.href}
              target="_blank"
              rel="noreferrer"
              className="text-primary inline-flex max-w-full items-center gap-1 hover:underline"
              aria-label={`访问${props.label} ${props.value}`}
            >
              <span className="truncate">{props.value}</span>
              <ExternalLink size={12} className="shrink-0" aria-hidden="true" />
            </a>
          ) : (
            props.value
          )}
        </TooltipTrigger>
        <TooltipContent className="max-w-xs break-all">{props.value}</TooltipContent>
      </Tooltip>
    </div>
  );
}
