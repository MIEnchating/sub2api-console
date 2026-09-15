import type { ReactElement } from "react";

import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

export function NewAPIFormSkeleton(props: { label: string; channel: boolean }): ReactElement {
  return (
    <div role="status" aria-label={props.label} aria-busy="true" className="min-w-0 w-full">
      <span className="sr-only">{props.label}</span>
      <Card aria-hidden="true" className="w-full">
        <div className="border-b bg-muted/20 px-4 py-4 sm:px-5">
          <Skeleton className="h-5 w-36" />
          {props.channel ? (
            <div className="mx-auto mt-4 grid max-w-2xl grid-cols-[minmax(0,1fr)_minmax(2rem,6rem)_minmax(0,1fr)] items-start">
              <div className="grid justify-items-center gap-1.5">
                <Skeleton className="size-7 rounded-full" />
                <Skeleton className="h-4 w-16" />
              </div>
              <Skeleton className="mt-3 h-px w-full" />
              <div className="grid justify-items-center gap-1.5">
                <Skeleton className="size-7 rounded-full" />
                <Skeleton className="h-4 w-16" />
              </div>
            </div>
          ) : null}
        </div>
        {props.channel ? (
          <>
            <div
              data-channel-credentials-layout=""
              className="grid min-w-0 divide-y lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.65fr)] lg:divide-x lg:divide-y-0"
            >
              <FormFieldsSkeleton fields={2} className="gap-4 p-4 sm:p-5" />
              <FormFieldsSkeleton fields={1} className="gap-4 bg-muted/10 p-4 sm:p-5" />
            </div>
            <div className="flex justify-end border-t px-4 py-3 sm:px-5">
              <Skeleton className="h-8 w-28" />
            </div>
          </>
        ) : (
          <div data-slot="platform-details-grid" className="grid gap-px bg-border sm:grid-cols-2">
            {[0, 1, 2, 3].map((item) => (
              <div key={item} className="grid gap-1.5 bg-background p-4">
                <Skeleton className="h-4 w-20" />
                <Skeleton className="h-5 w-40" />
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
