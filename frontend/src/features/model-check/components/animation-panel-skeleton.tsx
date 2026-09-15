import type { ReactElement } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { AnimationAccountsSkeleton } from "./animation-accounts-skeleton";

export function AnimationPanelSkeleton(): ReactElement {
  return (
    <div
      role="status"
      aria-label="正在读取动画检测"
      aria-busy="true"
      data-slot="page-loading-skeleton"
      className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border bg-card"
    >
      <div aria-hidden="true" className="flex shrink-0 gap-4 border-b px-3 py-3">
        <Skeleton className="h-4 w-16" />
        <Skeleton className="h-4 w-20" />
      </div>
      <div aria-hidden="true" className="grid shrink-0 gap-3 border-b p-3">
        <div className="flex min-w-0 gap-2 overflow-hidden">
          <Skeleton className="h-8 w-64 shrink-0" />
          <Skeleton className="h-8 w-24 shrink-0" />
          <Skeleton className="h-8 w-24 shrink-0" />
        </div>
        <div className="flex min-w-0 flex-wrap gap-2">
          <Skeleton className="h-8 w-40 max-w-full" />
          <Skeleton className="h-8 w-24" />
        </div>
      </div>
      <div aria-hidden="true" className="min-h-0 flex-1 overflow-y-auto p-3 sm:p-4">
        <AnimationAccountsSkeleton />
      </div>
      <div
        aria-hidden="true"
        className="flex shrink-0 items-center justify-between gap-3 border-t p-3"
      >
        <Skeleton className="h-4 w-20" />
        <Skeleton className="h-8 w-32" />
      </div>
    </div>
  );
}
