export type CalendarPopoverPlacement = "top start" | "bottom start";

export function getCalendarPopoverPlacement(
  trigger: HTMLElement | null,
  estimatedHeight = 320,
): CalendarPopoverPlacement {
  if (!trigger) return "bottom start";
  const triggerRect = trigger.getBoundingClientRect();
  const boundary = trigger.closest<HTMLElement>(
    '[data-slot="dialog-body"], [data-slot="sheet-content"]',
  );
  const boundaryRect = boundary?.getBoundingClientRect();
  const boundaryTop = boundaryRect?.top ?? 0;
  const boundaryBottom =
    boundaryRect?.bottom ??
    (typeof window === "undefined" ? triggerRect.bottom : window.innerHeight);
  const spaceAbove = triggerRect.top - boundaryTop;
  const spaceBelow = boundaryBottom - triggerRect.bottom;
  if (spaceBelow >= estimatedHeight) return "bottom start";
  return spaceAbove > spaceBelow ? "top start" : "bottom start";
}
