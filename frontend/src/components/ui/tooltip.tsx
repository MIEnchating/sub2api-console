/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";
import type { CSSProperties } from "react";

import { cn } from "@/lib/utils";

export const tooltipContentStyles =
  "bg-popover text-popover-foreground border-border z-50 inline-flex w-fit max-w-xs origin-(--transform-origin) items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs whitespace-normal [overflow-wrap:anywhere] shadow-md transition-[opacity,scale] duration-100 ease-out data-instant:transition-none data-ending-style:scale-95 data-ending-style:opacity-0 data-starting-style:scale-95 data-starting-style:opacity-0 has-data-[slot=kbd]:pr-1.5 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm";

export const tooltipArrowStyles =
  "bg-popover fill-popover z-50 size-2.5 translate-y-[calc(-50%-2px)] rotate-45 rounded-[2px] data-[side=bottom]:top-1 data-[side=inline-end]:top-1/2! data-[side=inline-end]:-left-1 data-[side=inline-end]:-translate-y-1/2 data-[side=inline-start]:top-1/2! data-[side=inline-start]:-right-1 data-[side=inline-start]:-translate-y-1/2 data-[side=left]:top-1/2! data-[side=left]:-right-1 data-[side=left]:-translate-y-1/2 data-[side=right]:top-1/2! data-[side=right]:-left-1 data-[side=right]:-translate-y-1/2 data-[side=top]:-bottom-2.5";

export const tooltipDefaultSideOffset = 8;
export const tooltipDefaultDelay = 300;

function TooltipProvider({
  delay = tooltipDefaultDelay,
  ...props
}: TooltipPrimitive.Provider.Props) {
  return <TooltipPrimitive.Provider data-slot="tooltip-provider" delay={delay} {...props} />;
}

function Tooltip({ ...props }: TooltipPrimitive.Root.Props) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />;
}

function TooltipTrigger({ ...props }: TooltipPrimitive.Trigger.Props) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

function TooltipContent({
  className,
  side = "top",
  sideOffset = tooltipDefaultSideOffset,
  align = "center",
  alignOffset = 0,
  children,
  ...props
}: TooltipPrimitive.Popup.Props &
  Pick<TooltipPrimitive.Positioner.Props, "align" | "alignOffset" | "side" | "sideOffset">) {
  // Keep the visual gap, but let pointer movement through it belong to this
  // popup instead of triggering neighbouring tooltips underneath.
  const hoverGap = typeof sideOffset === "number" ? Math.max(0, sideOffset) : 0;
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Positioner
        align={align}
        alignOffset={alignOffset}
        side={side}
        sideOffset={sideOffset}
        className="isolate z-50 max-w-[min(var(--available-width,100vw),calc(100vw-1rem))]"
        style={{ "--tooltip-hover-gap": `${hoverGap}px` } as CSSProperties}
      >
        <TooltipPrimitive.Popup
          data-slot="tooltip-content"
          className={cn(
            tooltipContentStyles,
            "relative select-text",
            hoverGap > 0 &&
              "before:absolute before:content-[''] data-[side=top]:before:inset-x-0 data-[side=top]:before:top-full data-[side=top]:before:h-[calc(var(--tooltip-hover-gap)+1px)] data-[side=bottom]:before:inset-x-0 data-[side=bottom]:before:bottom-full data-[side=bottom]:before:h-[calc(var(--tooltip-hover-gap)+1px)] data-[side=left]:before:inset-y-0 data-[side=left]:before:left-full data-[side=left]:before:w-[calc(var(--tooltip-hover-gap)+1px)] data-[side=right]:before:inset-y-0 data-[side=right]:before:right-full data-[side=right]:before:w-[calc(var(--tooltip-hover-gap)+1px)]",
            className,
          )}
          {...props}
        >
          {children}
          <TooltipPrimitive.Arrow className={tooltipArrowStyles} />
        </TooltipPrimitive.Popup>
      </TooltipPrimitive.Positioner>
    </TooltipPrimitive.Portal>
  );
}

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider };
