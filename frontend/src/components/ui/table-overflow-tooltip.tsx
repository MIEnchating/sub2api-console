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
"use client";

import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from "react";

import { observeTableOverflow } from "@/components/ui/table-overflow-observer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export function isElementOverflowing(
  element: Pick<HTMLElement, "clientHeight" | "clientWidth" | "scrollHeight" | "scrollWidth">,
): boolean {
  return element.scrollWidth > element.clientWidth || element.scrollHeight > element.clientHeight;
}

function hasFocusableChild(element: HTMLElement): boolean {
  return Array.from(
    element.querySelectorAll<HTMLElement>(
      'a[href],button,input,select,textarea,[tabindex],[contenteditable="true"]',
    ),
  ).some(
    (child) =>
      child.tabIndex >= 0 &&
      !child.matches(":disabled") &&
      child.getClientRects().length > 0 &&
      getComputedStyle(child).visibility !== "hidden",
  );
}

export function TableOverflowTooltip(props: {
  children: ReactNode;
  content: ReactNode;
  className?: string;
}) {
  const tooltipID = useId();
  const triggerRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const [focusable, setFocusable] = useState(false);
  const refreshOverflow = useCallback((): void => {
    const element = triggerRef.current;
    if (!element) return;
    const overflowing = isElementOverflowing(element);
    setFocusable(overflowing && !hasFocusableChild(element));
    if (!overflowing) setOpen(false);
  }, []);

  useEffect(() => {
    const element = triggerRef.current;
    if (!element) return;
    return observeTableOverflow(element, refreshOverflow);
  }, [refreshOverflow]);

  useEffect(refreshOverflow, [refreshOverflow, props.children, props.content, props.className]);

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setOpen(false);
      return;
    }
    setOpen(triggerRef.current !== null && isElementOverflowing(triggerRef.current));
  }

  return (
    <Tooltip open={open} onOpenChange={handleOpenChange}>
      <TooltipTrigger
        render={
          <div
            ref={triggerRef}
            tabIndex={focusable ? 0 : -1}
            aria-describedby={open ? tooltipID : undefined}
            className={cn(
              "block min-w-0 max-w-full truncate rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
              props.className,
            )}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent id={tooltipID} role="tooltip" className="max-w-xs break-all">
        {props.content}
      </TooltipContent>
    </Tooltip>
  );
}
