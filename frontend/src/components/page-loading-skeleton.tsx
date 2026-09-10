import type { ReactElement } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

type PageLoadingSkeletonProps = {
  label: string;
  variant?: "table" | "form" | "list";
  className?: string;
};

const rows = [0, 1, 2, 3, 4, 5];
const fields = [0, 1, 2, 3];

export function PageLoadingSkeleton(props: PageLoadingSkeletonProps): ReactElement {
  const variant = props.variant ?? "table";
  return (
    <div
      role="status"
      aria-label={props.label}
      aria-busy="true"
      data-slot="page-loading-skeleton"
      className={cn("min-h-0 min-w-0 w-full", props.className)}
    >
      <span className="sr-only">{props.label}</span>
      {variant === "form" ? (
        <div aria-hidden="true" className="grid gap-4 md:grid-cols-2">
          {[0, 1].map((card) => (
            <div key={card} className="grid content-start gap-5 rounded-xl border p-4">
              <Skeleton className="h-5 w-28" />
              {fields.map((field) => (
                <div key={field} className="grid gap-2">
                  <Skeleton className="h-3 w-24" />
                  <Skeleton className="h-9 w-full" />
                </div>
              ))}
            </div>
          ))}
        </div>
      ) : (
        <div aria-hidden="true" className="overflow-hidden rounded-xl border">
          {variant === "table" ? (
            <div className="flex gap-3 border-b p-3">
              <Skeleton className="h-8 w-60 max-w-full" />
              <Skeleton className="ml-auto h-8 w-20" />
            </div>
          ) : null}
          <div className="divide-y">
            {rows.map((row) => (
              <div key={row} className="flex min-h-16 items-center gap-4 px-4 py-3">
                <Skeleton className="size-5 shrink-0" />
                <div className="grid min-w-0 flex-1 gap-2">
                  <Skeleton className="h-4 w-2/3" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
                {variant === "table" ? <Skeleton className="hidden h-4 w-1/4 sm:block" /> : null}
                <Skeleton className="h-6 w-12 shrink-0" />
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
