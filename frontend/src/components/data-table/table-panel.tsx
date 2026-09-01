import type * as React from "react";

import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function DataTablePanel(props: React.ComponentProps<typeof Card>) {
  return (
    <Card
      {...props}
      data-table-panel=""
      className={cn("min-h-0 min-w-0 gap-0 py-0", props.className)}
    />
  );
}
