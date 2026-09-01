import type * as React from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function SegmentedControl(props: React.ComponentProps<"div">) {
  return (
    <div
      {...props}
      data-slot="segmented-control"
      className={cn(
        "bg-muted/40 inline-flex w-fit max-w-full items-center gap-1 rounded-md border p-1",
        props.className,
      )}
    />
  );
}

export function SegmentedControlItem(
  props: Omit<React.ComponentProps<typeof Button>, "variant" | "size"> & {
    selected: boolean;
  },
) {
  const { selected, className, ...buttonProps } = props;
  return (
    <Button
      {...buttonProps}
      data-slot="segmented-control-item"
      size="sm"
      variant={selected ? "secondary" : "ghost"}
      aria-pressed={selected}
      className={cn("h-7", selected && "bg-background shadow-xs", className)}
    />
  );
}
