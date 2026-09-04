import type * as React from "react";

import { cn } from "@/lib/utils";

export function TableFilterToolbar(props: React.ComponentProps<"div">) {
  return (
    <div
      {...props}
      data-slot="table-filter-toolbar"
      className={cn("flex w-full min-w-0 shrink-0 flex-wrap items-center gap-2", props.className)}
    />
  );
}
