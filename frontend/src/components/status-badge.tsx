import type { LucideIcon } from "lucide-react";
import type * as React from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { statusToneClasses, type StatusTone } from "@/lib/status-styles";

export type StatusVariant = StatusTone;

const sizeMap = {
  sm: "h-5 gap-1 px-2 text-xs leading-4",
  md: "h-5 gap-1 px-2 text-xs leading-4",
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
  const { label, icon: Icon, pulse, variant, size, title, className, ...spanProps } = props;

  const badge = (
    <span
      {...spanProps}
      data-slot="status-badge"
      className={cn(
        "inline-flex w-fit max-w-full min-w-0 shrink items-center rounded-4xl border font-medium tracking-normal whitespace-nowrap transition-colors",
        sizeMap[size ?? "sm"],
        statusToneClasses[variant ?? "neutral"],
        pulse && "animate-pulse",
        className,
      )}
    >
      {Icon && <Icon className="size-3 shrink-0" aria-hidden="true" />}
      <span className="min-w-0 truncate leading-normal">{label}</span>
    </span>
  );
  if (!title) return badge;
  return (
    <Tooltip>
      <TooltipTrigger render={badge} />
      <TooltipContent>{title}</TooltipContent>
    </Tooltip>
  );
}
