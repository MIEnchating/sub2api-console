import type { ReactElement } from "react";
import { Skeleton } from "@/components/ui/skeleton";

export function AnimationHistorySkeleton(): ReactElement {
  return (
    <div
      role="status"
      aria-label="正在读取自定义检测记录"
      aria-busy="true"
      data-slot="page-loading-skeleton"
      className="col-span-full grid min-h-0 min-w-0 gap-3 md:grid-cols-3"
    >
      {[0, 1, 2].map((index) => (
        <div
          key={index}
          aria-hidden="true"
          className="flex min-h-40 min-w-0 flex-col gap-2 rounded-lg border p-2"
        >
          <div className="flex min-h-24 flex-1 items-center justify-center rounded-md bg-muted/10">
            <Skeleton className="size-8" />
          </div>
          <Skeleton className="h-4 w-1/2" />
          <Skeleton className="h-3 w-3/4" />
        </div>
      ))}
    </div>
  );
}

export function AnimationAccountsSkeleton(): ReactElement {
  return (
    <div
      role="status"
      aria-label="正在读取账号"
      aria-busy="true"
      data-slot="page-loading-skeleton"
      className="grid min-w-0 grid-cols-[repeat(auto-fill,minmax(min(100%,20rem),1fr))] items-start gap-3"
    >
      {[0, 1, 2, 3].map((index) => (
        <div
          key={index}
          aria-hidden="true"
          className="flex h-auto min-w-0 flex-col overflow-hidden rounded-lg border border-border/70 bg-card"
        >
          <div className="grid h-24 shrink-0 content-center gap-1.5 px-3">
            <div className="flex items-center gap-2">
              <Skeleton className="size-4 shrink-0" />
              <Skeleton className="h-4 w-2/3" />
            </div>
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-3 w-full" />
          </div>
          <Skeleton className="h-[180px] w-full shrink-0 rounded-none" />
          <div className="flex h-14 shrink-0 items-center gap-2 px-3">
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="ml-auto h-8 w-20" />
          </div>
          <div className="flex h-10 shrink-0 items-center gap-2 border-t px-3">
            <Skeleton className="h-4 w-16" />
            <Skeleton className="ml-auto size-8" />
          </div>
        </div>
      ))}
    </div>
  );
}
