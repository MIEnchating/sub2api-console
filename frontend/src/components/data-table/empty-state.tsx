import type { ReactNode } from "react";

import { TableCell, TableRow } from "@/components/ui/table";

export function TableEmptyState(props: { columns: number; children: ReactNode }) {
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={props.columns} overflowTooltip={false} className="p-0 whitespace-normal">
        <div
          data-slot="table-empty-state"
          className="text-muted-foreground sticky left-0 grid min-h-28 w-[100cqi] max-w-full place-items-center px-4 py-6 text-center [overflow-wrap:anywhere]"
        >
          {props.children}
        </div>
      </TableCell>
    </TableRow>
  );
}
