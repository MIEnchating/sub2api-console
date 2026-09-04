import type * as React from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function SegmentedControl(props: React.ComponentProps<"div">) {
  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    props.onKeyDown?.(event);
    if (event.defaultPrevented || props.role !== "tablist") return;
    if (!["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"].includes(event.key))
      return;
    const tabs = Array.from(
      event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]:not(:disabled)'),
    );
    const current = (event.target as HTMLElement).closest<HTMLButtonElement>('[role="tab"]');
    const currentIndex = current ? tabs.indexOf(current) : -1;
    if (currentIndex < 0 || tabs.length === 0) return;
    event.preventDefault();
    let nextIndex = currentIndex;
    if (event.key === "Home") nextIndex = 0;
    else if (event.key === "End") nextIndex = tabs.length - 1;
    else if (event.key === "ArrowRight" || event.key === "ArrowDown")
      nextIndex = (currentIndex + 1) % tabs.length;
    else nextIndex = (currentIndex - 1 + tabs.length) % tabs.length;
    tabs[nextIndex]?.focus();
    tabs[nextIndex]?.click();
  }
  return (
    <div
      {...props}
      onKeyDown={handleKeyDown}
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
  let tabIndex = props.tabIndex;
  if (props.role === "tab") tabIndex = selected ? 0 : -1;
  return (
    <Button
      {...buttonProps}
      data-slot="segmented-control-item"
      size="sm"
      variant={selected ? "secondary" : "ghost"}
      aria-pressed={props.role === "tab" ? undefined : selected}
      aria-selected={props.role === "tab" ? selected : props["aria-selected"]}
      tabIndex={tabIndex}
      className={cn("h-7", selected && "bg-background shadow-xs", className)}
    />
  );
}
