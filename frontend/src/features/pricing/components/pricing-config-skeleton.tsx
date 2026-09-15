import type { ReactElement } from "react";

import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { PricingConfigLayout } from "./pricing-config-layout";

export function PricingConfigSkeleton(): ReactElement {
  return (
    <div role="status" aria-label="正在读取价格设置" aria-busy="true" data-testid="pricing-loading">
      <span className="sr-only">正在读取价格设置</span>
      <PricingConfigLayout aria-hidden="true">
        <Card size="sm" data-testid="pricing-settings-skeleton">
          <CardHeader className="bg-muted/20 grid-cols-1 gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <Skeleton className="size-8 shrink-0" />
              <div className="grid min-w-0 flex-1 gap-1.5">
                <Skeleton className="h-5 w-28" />
                <Skeleton className="h-5 w-48" />
              </div>
            </div>
            <div className="bg-background flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5">
              <Skeleton className="h-5 w-16" />
              <Skeleton className="h-5 w-9 rounded-full" />
            </div>
          </CardHeader>
          <CardContent className="grid divide-y p-0 lg:grid-cols-3 lg:divide-x lg:divide-y-0 xl:grid-cols-1 xl:divide-x-0 xl:divide-y">
            {[0, 1, 2].map((field) => (
              <div
                key={field}
                className="grid min-w-0 grid-cols-[minmax(0,1fr)_8rem] items-center gap-2.5 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] lg:grid-cols-1 lg:items-start lg:py-4"
              >
                <Skeleton className="h-5 w-24" />
                <Skeleton data-slot="skeleton-control" className="h-8 w-full rounded-lg" />
              </div>
            ))}
          </CardContent>
          <div className="grid gap-2 border-t px-3 py-3">
            <Skeleton className="h-5 w-full" />
            <Skeleton className="h-5 w-28" />
          </div>
        </Card>

        <Card size="sm" data-testid="pricing-exchange-skeleton">
          <CardHeader className="bg-muted/20 flex flex-wrap items-start justify-between gap-3 sm:flex-row sm:items-center">
            <div className="flex min-w-0 items-center gap-3">
              <Skeleton className="size-8 shrink-0" />
              <div className="grid min-w-0 gap-1.5">
                <Skeleton className="h-5 w-36" />
                <Skeleton className="h-5 w-72" />
              </div>
            </div>
            <Skeleton className="h-8 w-28" />
          </CardHeader>
          <CardContent>
            <div className="overflow-hidden rounded-lg border">
              <div className="bg-muted/30 flex min-h-12 flex-wrap items-center gap-2 border-b px-3 py-3">
                <Skeleton className="h-4 w-10" />
                <Skeleton data-slot="skeleton-control" className="h-8 max-w-64 flex-1 basis-36" />
                <Skeleton className="h-5 w-16" />
                <div className="ml-auto flex gap-1">
                  <Skeleton className="size-8" />
                  <Skeleton className="size-8" />
                </div>
              </div>
              <div className="grid gap-2 px-3 py-2.5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-2 2xl:grid-cols-3">
                {[0, 1, 2, 3].map((group) => (
                  <div key={group} className="min-w-0 overflow-hidden rounded-lg border">
                    <div className="flex min-h-12 items-center gap-2.5 px-3 py-2.5">
                      <Skeleton className="size-4 shrink-0" />
                      <Skeleton className="h-5 flex-1" />
                      <Skeleton className="h-4 w-12" />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      </PricingConfigLayout>
    </div>
  );
}
