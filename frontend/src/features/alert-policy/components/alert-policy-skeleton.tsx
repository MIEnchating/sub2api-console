import type { ReactElement } from "react";

import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { alertRuleGroups, recoveryNotificationFields, routingDegradedFields } from "../constants";
import { AlertPolicyLayout } from "./alert-policy-layout";

function SwitchSkeleton(): ReactElement {
  return (
    <div className="flex min-h-11 min-w-0 items-center justify-between gap-3 py-2">
      <Skeleton className="h-4 w-28 max-w-full" />
      <Skeleton className="h-5 w-9 shrink-0 rounded-full" />
    </div>
  );
}

function DetectionSkeleton(): ReactElement {
  return (
    <Card data-testid="alert-detection-skeleton">
      <CardHeader>
        <Skeleton className="h-5 w-28" />
      </CardHeader>
      <CardContent className="py-3">
        <div className="mb-4 rounded-lg border px-3 py-1">
          <SwitchSkeleton />
        </div>
        <Skeleton className="mb-3 h-5 w-20" />
        <div className="grid gap-3">
          {alertRuleGroups.map((group) => (
            <div key={group.label} className="rounded-lg border px-3 py-1.5">
              <Skeleton className="mb-1 h-4 w-20" />
              <div className="grid gap-x-5 sm:grid-cols-2">
                {group.fields.map((field) => (
                  <SwitchSkeleton key={field.name} />
                ))}
              </div>
            </div>
          ))}
        </div>
        <div className="mt-3 rounded-lg border px-3 py-1.5">
          <SwitchSkeleton />
          <div className="mt-1 grid gap-x-5 border-t pt-1 sm:grid-cols-2">
            {routingDegradedFields.map((field) => (
              <SwitchSkeleton key={field.value} />
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function ThresholdSkeleton(): ReactElement {
  return (
    <Card data-testid="alert-threshold-skeleton">
      <CardHeader>
        <Skeleton className="h-5 w-28" />
      </CardHeader>
      <CardContent className="grid gap-3 py-3 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <div className="flex items-center justify-between gap-2">
            <Skeleton className="h-5 w-28" />
            <Skeleton className="h-8 w-24" />
          </div>
          <div className="mt-2 flex flex-wrap gap-2">
            {[0, 1, 2].map((index) => (
              <Skeleton key={index} data-slot="skeleton-control" className="h-8 w-28" />
            ))}
          </div>
        </div>
        <FormFieldsSkeleton fields={1} />
        <FormFieldsSkeleton fields={1} />
        <FormFieldsSkeleton fields={1} className="sm:col-span-2" />
      </CardContent>
    </Card>
  );
}

function NotificationSkeleton(): ReactElement {
  return (
    <Card data-testid="alert-notification-skeleton">
      <CardHeader className="flex flex-wrap items-center justify-between gap-2">
        <Skeleton className="h-5 w-28" />
        <div className="flex items-center gap-1.5">
          <Skeleton className="h-5 w-24" />
          <Skeleton className="size-8" />
        </div>
      </CardHeader>
      <CardContent className="py-3">
        <div className="grid gap-x-5 sm:grid-cols-2">
          <SwitchSkeleton />
          <SwitchSkeleton />
        </div>
        <div className="mt-3 grid gap-4 border-t pt-3 sm:grid-cols-2 2xl:grid-cols-3">
          {[0, 1, 2].map((index) => (
            <FormFieldsSkeleton key={index} fields={1} />
          ))}
        </div>
        <div className="mt-3 rounded-lg border px-3 py-2">
          <Skeleton className="h-5 w-28" />
          <div className="mt-1 grid gap-x-5 sm:grid-cols-2">
            {recoveryNotificationFields.map((field) => (
              <SwitchSkeleton key={field.value} />
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function AlertPolicySkeleton(): ReactElement {
  return (
    <div role="status" aria-label="正在读取告警策略" aria-busy="true" className="min-w-0 w-full">
      <span className="sr-only">正在读取告警策略</span>
      <div aria-hidden="true">
        <AlertPolicyLayout detection={<DetectionSkeleton />}>
          <ThresholdSkeleton />
          <NotificationSkeleton />
        </AlertPolicyLayout>
      </div>
    </div>
  );
}
