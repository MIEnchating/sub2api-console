import type * as React from "react";

import { cn } from "@/lib/utils";

export type PageActionsProps = {
  children: React.ReactNode;
  className?: string;
};

export function PageActions(props: PageActionsProps) {
  return (
    <div
      data-slot="page-actions"
      className={cn(
        "flex flex-wrap items-center justify-end gap-2",
        props.className,
      )}
    >
      {props.children}
    </div>
  );
}
