import type { ReactElement, ReactNode } from "react";
import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { settingsLayout } from "./settings-layout";
import { SettingsFooter } from "./settings-footer";

function SettingsSkeletonCard(props: {
  title: string;
  label: string;
  contentClassName: string;
  children: ReactNode;
}): ReactElement {
  return (
    <Card
      size="sm"
      className="h-full min-h-0 min-w-0"
      role="status"
      aria-label={props.label}
      aria-busy="true"
    >
      <CardHeader className="shrink-0">
        <CardTitle>{props.title}</CardTitle>
        <Skeleton className="h-4 w-full max-w-sm" />
      </CardHeader>
      <CardContent data-slot="settings-scroll" className={props.contentClassName}>
        {props.children}
      </CardContent>
      <SettingsFooter>
        <Skeleton className="h-8 w-28" />
        <Skeleton className="h-8 w-28" />
      </SettingsFooter>
    </Card>
  );
}

export function ConnectionSettingsSkeleton(): ReactElement {
  return (
    <SettingsSkeletonCard
      title="Sub2API 连接"
      label="正在读取连接设置"
      contentClassName={settingsLayout.connectionFields}
    >
      <div className="flex items-center justify-between gap-3 lg:col-span-2" aria-hidden="true">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-8 w-48" />
      </div>
      <FormFieldsSkeleton fields={1} className="lg:col-span-2" />
      <FormFieldsSkeleton fields={1} />
      <FormFieldsSkeleton fields={1} />
    </SettingsSkeletonCard>
  );
}

export function NotificationSettingsSkeleton(): ReactElement {
  return (
    <SettingsSkeletonCard
      title="QQBot 通知接入"
      label="正在读取通知设置"
      contentClassName={settingsLayout.notificationFields}
    >
      <div
        className={settingsLayout.notificationCredentials}
        data-testid="notification-credentials"
      >
        <FormFieldsSkeleton fields={1} />
        <FormFieldsSkeleton fields={1} />
      </div>
      <div
        className={settingsLayout.notificationDestination}
        data-testid="notification-destination"
      >
        <FormFieldsSkeleton fields={1} />
        <FormFieldsSkeleton fields={1} />
      </div>
    </SettingsSkeletonCard>
  );
}

export function LogCleanupSettingsSkeleton(): ReactElement {
  return (
    <SettingsSkeletonCard
      title="日志保留"
      label="正在读取日志保留设置"
      contentClassName={settingsLayout.logFields}
    >
      <div className={settingsLayout.logControls} aria-hidden="true">
        <div className="flex items-center justify-between gap-3">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-4 w-7 rounded-full" />
        </div>
        <div className="flex items-center justify-between gap-3">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-8 w-20" />
        </div>
        <div className="grid gap-1.5 border-t pt-3 sm:col-span-2">
          <Skeleton className="h-4 w-36" />
          <Skeleton className="h-3 w-full max-w-64" />
        </div>
      </div>
    </SettingsSkeletonCard>
  );
}
