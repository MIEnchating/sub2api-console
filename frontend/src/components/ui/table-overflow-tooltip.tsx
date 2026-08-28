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

import { useRef, useState, type ReactNode } from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export function isElementOverflowing(
  element: Pick<HTMLElement, "clientHeight" | "clientWidth" | "scrollHeight" | "scrollWidth">,
): boolean {
  return element.scrollWidth > element.clientWidth || element.scrollHeight > element.clientHeight;
}

export function TableOverflowTooltip(props: {
  children: ReactNode;
  content: ReactNode;
  className?: string;
}) {
  const triggerRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);

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
            className={cn("block min-w-0 max-w-full truncate", props.className)}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent className="max-w-xs break-all">{props.content}</TooltipContent>
    </Tooltip>
  );
}
