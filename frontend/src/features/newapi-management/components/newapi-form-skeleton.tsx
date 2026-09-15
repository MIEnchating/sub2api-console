import type { ReactElement } from "react";

import { ChannelFormColumns, ChannelFormFooter } from "./channel-form-layout";
import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { Card, CardAction, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

export function NewAPIFormSkeleton(props: { label: string; channel: boolean }): ReactElement {
  return (
    <div role="status" aria-label={props.label} aria-busy="true" className="min-w-0 w-full">
      <span className="sr-only">{props.label}</span>
      <Card aria-hidden="true" className={props.channel ? "@container/channel w-full" : "w-full"}>
        <CardHeader>
          <div className="grid gap-1">
            <Skeleton className="h-5 w-36" />
            <Skeleton className="h-4 w-20" />
          </div>
          {props.channel ? (
            <CardAction className="gap-3">
              <Skeleton className="size-6 rounded-full" />
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-px w-6" />
              <Skeleton className="size-6 rounded-full" />
              <Skeleton className="h-4 w-16" />
            </CardAction>
          ) : null}
        </CardHeader>
        {props.channel ? (
          <>
            <ChannelFormColumns kind="credentials">
              <div className="col-span-full grid gap-1.5">
                <Skeleton className="h-4 w-16" />
                <Skeleton className="h-8 w-64 max-w-full" />
              </div>
              <div className="grid content-start gap-4">
                <FormFieldsSkeleton fields={1} />
              </div>
              <div className="grid content-start gap-3">
                <FormFieldsSkeleton fields={1} />
                <div className="flex flex-wrap gap-2">
                  <Skeleton className="h-4 w-24" />
                  <Skeleton className="h-4 w-40" />
                </div>
              </div>
            </ChannelFormColumns>
            <ChannelFormFooter note="">
              <Skeleton className="h-8 w-28" />
            </ChannelFormFooter>
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
