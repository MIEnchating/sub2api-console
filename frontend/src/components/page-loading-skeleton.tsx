import type { ReactElement } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { cn } from "@/lib/utils";

type PageLoadingSkeletonProps = {
  label: string;
  variant?: "table" | "form" | "list";
  fill?: boolean;
  framed?: boolean;
  panels?: number;
  className?: string;
};

const rows = [0, 1, 2, 3, 4, 5];

export function PageLoadingSkeleton(props: PageLoadingSkeletonProps): ReactElement {
  const variant = props.variant ?? "table";
  const framed = props.framed ?? true;
  const panels = props.panels ?? 1;
  return (
    <div
      role="status"
      aria-label={props.label}
      aria-busy="true"
      data-slot="page-loading-skeleton"
      className={cn(
        "flex min-h-0 min-w-0 w-full flex-col gap-3",
        props.fill && "h-full flex-1 overflow-hidden",
        props.className,
      )}
    >
      <span className="sr-only">{props.label}</span>
      {variant === "form" ? (
        <div
          aria-hidden="true"
          className={cn("grid min-w-0 gap-3", panels > 1 && "lg:grid-cols-2")}
        >
          {Array.from({ length: panels }, (_, card) => (
            <div key={card} className="min-w-0 overflow-hidden rounded-lg border bg-card">
              <div className="grid gap-1.5 border-b px-4 py-3">
                <Skeleton className="h-5 w-28" />
                <Skeleton className="h-4 w-3/4 max-w-sm" />
              </div>
              <FormFieldsSkeleton className="p-4" />
            </div>
          ))}
        </div>
      ) : (
        <>
          {variant === "table" && framed ? (
            <div aria-hidden="true" className="flex min-w-0 shrink-0 gap-2">
              <Skeleton className="h-8 w-56 min-w-0" />
              <Skeleton className="ml-auto size-8 shrink-0" />
            </div>
          ) : null}
          <div
            aria-hidden="true"
            className={cn(
              "flex min-h-0 min-w-0 flex-col overflow-hidden",
              framed && "rounded-lg border bg-card",
              props.fill && "flex-1",
            )}
          >
            {variant === "table" ? (
              <div
                data-slot="skeleton-table-header"
                className="flex h-10 shrink-0 items-center gap-4 border-b bg-muted/30 px-3"
              >
                <Skeleton className="h-4 w-1/3" />
                <Skeleton className="h-4 w-1/4" />
                <Skeleton className="ml-auto h-4 w-12" />
              </div>
            ) : null}
            <div data-slot="skeleton-rows" className="min-h-0 overflow-hidden divide-y">
              {rows.map((row) => (
                <div
                  key={row}
                  className={cn(
                    "flex min-w-0 items-center gap-3 px-3",
                    variant === "table" ? "h-15" : "h-16",
                  )}
                >
                  <Skeleton className="size-4 shrink-0" />
                  <div className="grid min-w-0 flex-1 gap-1.5">
                    <Skeleton className="h-4 w-2/3 max-w-56" />
                    <Skeleton className="h-3 w-1/3 max-w-32" />
                  </div>
                  <Skeleton className="h-5 w-12 shrink-0" />
                </div>
              ))}
            </div>
            {variant === "table" ? (
              <div
                data-slot="skeleton-pagination"
                className="mt-auto flex shrink-0 items-center justify-end gap-2 border-t px-3 py-2.5 sm:px-4 sm:py-3"
              >
                <Skeleton className="h-4 w-12" />
                <Skeleton className="h-8 w-16" />
                <Skeleton className="size-8" />
                <Skeleton className="size-8" />
              </div>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
}
