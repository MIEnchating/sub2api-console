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

import * as React from "react";

import { cn } from "@/lib/utils";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";

const TableOverflowTooltipContext = React.createContext(true);

type TableProps = React.ComponentProps<"table"> & {
  containerClassName?: string;
  overflowTooltip?: boolean;
};

function Table({ className, containerClassName, overflowTooltip = true, ...props }: TableProps) {
  return (
    <TableOverflowTooltipContext.Provider value={overflowTooltip}>
      <div
        data-slot="table-container"
        className={cn("relative w-full overflow-x-auto overflow-y-hidden", containerClassName)}
      >
        <table
          data-slot="table"
          data-overflow-tooltip={overflowTooltip ? "true" : "false"}
          className={cn(
            "w-full caption-bottom text-sm tabular-nums [&_td]:text-sm [&_td_*]:text-sm [&_th]:text-sm [&_th_*]:text-sm",
            overflowTooltip && "table-fixed",
            className,
          )}
          {...props}
        />
      </div>
    </TableOverflowTooltipContext.Provider>
  );
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn(
        "[background-color:var(--table-header)] [&_tr]:border-b [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:[background-color:var(--table-header)]",
        className,
      )}
      {...props}
    />
  );
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&>tr]:h-15 [&_tr:last-child]:border-0", className)}
      {...props}
    />
  );
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "group data-[state=selected]:bg-muted border-b transition-colors hover:[background-color:color-mix(in_oklch,var(--muted)_50%,var(--background))] has-aria-expanded:[background-color:color-mix(in_oklch,var(--muted)_50%,var(--background))]",
        className,
      )}
      {...props}
    />
  );
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "text-foreground h-10 px-2 text-left align-middle font-medium whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        className,
      )}
      {...props}
    />
  );
}

type TableCellProps = Omit<React.ComponentProps<"td">, "title"> & {
  overflowTooltip?: boolean;
  title?: string;
  tooltipContent?: React.ReactNode;
};

function TableCell({
  className,
  overflowTooltip,
  title,
  tooltipContent,
  children,
  ...props
}: TableCellProps) {
  const tableOverflowTooltip = React.useContext(TableOverflowTooltipContext);
  const enabled = overflowTooltip ?? tableOverflowTooltip;
  const content = tooltipContent ?? title ?? getTextContent(children);
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "min-w-0 p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        className,
      )}
      {...props}
    >
      {enabled && content ? (
        <TableOverflowTooltip content={content}>{children}</TableOverflowTooltip>
      ) : (
        children
      )}
    </td>
  );
}

function getTextContent(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(getTextContent).join("");
  return "";
}

export { Table, TableBody, TableCell, TableHead, TableHeader, TableRow };
