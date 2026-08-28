import type { LucideIcon } from "lucide-react";
import type * as React from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const textColorMap = {
  success: "text-success",
  warning: "text-warning",
  danger: "text-destructive",
  info: "text-info",
  purple: "text-purple-600 dark:text-purple-400",
  neutral: "text-muted-foreground",
} as const;

export type StatusVariant = keyof typeof textColorMap;

const sizeMap = {
  sm: "h-5 gap-1 px-1.5 text-sm leading-none",
  md: "h-5 gap-1 px-1.5 text-sm leading-none",
  lg: "h-6 gap-1.5 px-2 text-sm leading-none",
} as const;

export type StatusBadgeProps = Omit<React.HTMLAttributes<HTMLSpanElement>, "children"> & {
  label: string;
  icon?: LucideIcon;
  pulse?: boolean;
  variant?: StatusVariant;
  size?: keyof typeof sizeMap;
};

export function StatusBadge(props: StatusBadgeProps) {
  const Icon = props.icon;

  const badge = (
    <span
      data-slot="status-badge"
      className={cn(
        "inline-flex w-fit max-w-full min-w-0 shrink items-center rounded-4xl font-medium tracking-normal whitespace-nowrap transition-colors",
        sizeMap[props.size ?? "sm"],
        textColorMap[props.variant ?? "neutral"],
        props.pulse && "animate-pulse",
        props.className,
      )}
      aria-label={props["aria-label"]}
    >
      {Icon && <Icon className="size-3.5 shrink-0" />}
      <span className="min-w-0 truncate leading-normal">{props.label}</span>
    </span>
  );
  if (!props.title) return badge;
  return (
    <Tooltip>
      <TooltipTrigger render={badge} />
      <TooltipContent>{props.title}</TooltipContent>
    </Tooltip>
  );
}
