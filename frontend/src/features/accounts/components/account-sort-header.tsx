import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";

import { Button } from "@/components/ui/button";
import { TableHead } from "@/components/ui/table";
import {
  accountSortDirection,
  nextAccountSort,
  type AccountSort,
  type AccountSortColumn,
} from "@/features/accounts/lib/account-sort";
import { cn } from "@/lib/utils";

export function AccountSortTableHead(props: {
  label: string;
  column: AccountSortColumn;
  value: AccountSort;
  className?: string;
  onValueChange: (value: AccountSort) => void;
}) {
  const direction = accountSortDirection(props.value, props.column);
  let actionLabel = `按${props.label}升序排列`;
  let ariaSort: "ascending" | "descending" | "none" = "none";
  let SortIcon = ArrowUpDown;
  if (direction === "asc") actionLabel = `${props.label}当前升序，切换为降序`;
  if (direction === "desc") actionLabel = `${props.label}当前降序，恢复默认顺序`;
  if (direction === "asc") {
    ariaSort = "ascending";
    SortIcon = ArrowUp;
  }
  if (direction === "desc") {
    ariaSort = "descending";
    SortIcon = ArrowDown;
  }

  return (
    <TableHead className={props.className} aria-sort={ariaSort}>
      <Button
        type="button"
        variant="ghost"
        className="-ml-2 h-8 px-2 font-medium"
        aria-label={actionLabel}
        onClick={() => props.onValueChange(nextAccountSort(props.value, props.column))}
      >
        {props.label}
        <SortIcon
          className={cn("size-3.5", direction === null && "text-muted-foreground")}
          aria-hidden="true"
        />
      </Button>
    </TableHead>
  );
}
