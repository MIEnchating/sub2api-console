import type * as React from "react";

import { cn } from "@/lib/utils";

export type PageActionsProps = React.ComponentProps<"div">;

export function PageActions(props: PageActionsProps) {
  const { className, ...divProps } = props;
  return (
    <div
      data-slot="page-actions"
      {...divProps}
      className={cn("flex flex-wrap items-center justify-end gap-2", className)}
    />
  );
}
