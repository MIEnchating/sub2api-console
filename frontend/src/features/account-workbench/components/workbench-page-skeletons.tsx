import type { ReactElement, ReactNode } from "react";

import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { Skeleton } from "@/components/ui/skeleton";

function WorkbenchLoading(props: { label: string; children: ReactNode }): ReactElement {
  return (
    <div role="status" aria-label={props.label} aria-busy="true" className="min-w-0 w-full">
      <span className="sr-only">{props.label}</span>
      <div aria-hidden="true" className="grid min-w-0 gap-4">
        {props.children}
      </div>
    </div>
  );
}

function ChoiceSkeleton(): ReactElement {
  return (
    <div className="flex items-center gap-2">
      <Skeleton className="size-4 shrink-0" />
      <Skeleton className="h-5 w-48" />
    </div>
  );
}

export function WorkbenchImportSkeleton(): ReactElement {
  return (
    <WorkbenchLoading label="正在读取账号导入配置">
      <div
        data-slot="workbench-form-skeleton"
        className="grid min-w-0 gap-4 rounded-lg border bg-card p-4"
      >
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <FormFieldsSkeleton fields={1} className="sm:w-44" />
          <FormFieldsSkeleton fields={1} className="min-w-0 flex-1" />
        </div>
        <div className="grid min-w-0 gap-1.5">
          <Skeleton className="h-4 w-24" />
          <Skeleton data-slot="skeleton-textarea" className="h-64 w-full" />
        </div>
        <FormFieldsSkeleton fields={2} className="gap-4 sm:grid-cols-2" />
        <ChoiceSkeleton />
        <Skeleton className="h-5 w-3/4" />
        <div className="flex gap-2">
          <Skeleton className="h-8 w-28" />
          <Skeleton className="h-8 w-24" />
        </div>
      </div>
    </WorkbenchLoading>
  );
}

export function WorkbenchMaintenanceSkeleton(): ReactElement {
  return (
    <WorkbenchLoading label="正在读取账号维护设置">
      <div
        data-slot="workbench-form-skeleton"
        className="grid min-w-0 gap-4 rounded-lg border bg-card p-4"
      >
        <Skeleton className="h-5 w-32" />
        <Skeleton className="h-5 w-3/4" />
        <ChoiceSkeleton />
        <div data-slot="maintenance-parameters" className="grid gap-3 sm:grid-cols-2">
          <FormFieldsSkeleton fields={1} />
          <FormFieldsSkeleton fields={1} />
        </div>
        <div className="min-w-0 space-y-2">
          <Skeleton data-slot="maintenance-group-label" className="h-5 w-24" />
          <div
            data-slot="maintenance-group-options"
            className="grid max-h-44 gap-2 overflow-y-auto rounded-md border p-3 sm:grid-cols-2"
          >
            <ChoiceSkeleton />
            <ChoiceSkeleton />
          </div>
        </div>
        <ChoiceSkeleton />
        <FormFieldsSkeleton fields={1} />
        <div className="flex flex-wrap gap-2">
          <Skeleton className="h-8 w-28" />
          <Skeleton className="h-8 w-32" />
        </div>
      </div>
    </WorkbenchLoading>
  );
}

export function WorkbenchTemplatesSkeleton(): ReactElement {
  return (
    <WorkbenchLoading label="正在读取账号配置模板">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Skeleton className="h-5 w-full max-w-xl" />
        <Skeleton className="h-8 w-28" />
      </div>
      <div data-slot="workbench-template-grid" className="grid min-w-0 gap-3 lg:grid-cols-2">
        {[0, 1].map((item) => (
          <div
            key={item}
            data-slot="workbench-template-card"
            className="grid min-w-0 gap-3 rounded-lg border bg-card p-4"
          >
            <div className="flex justify-between gap-2">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="size-8" />
            </div>
            <div className="grid gap-2">
              {[0, 1, 2, 3].map((row) => (
                <div key={row} data-slot="workbench-template-detail" className="flex gap-3">
                  <Skeleton className="h-5 w-20" />
                  <Skeleton className="h-5 flex-1" />
                </div>
              ))}
            </div>
            <div data-slot="workbench-template-actions" className="flex flex-wrap gap-2">
              <Skeleton className="h-8 w-16" />
              <Skeleton className="h-8 w-16" />
            </div>
          </div>
        ))}
      </div>
    </WorkbenchLoading>
  );
}

export function WorkbenchSecuritySkeleton(): ReactElement {
  return (
    <WorkbenchLoading label="正在读取安全设置账号">
      <Skeleton className="h-6 w-32" />
      <div data-slot="security-selectors" className="grid min-w-0 gap-4 sm:grid-cols-2">
        <FormFieldsSkeleton fields={1} />
        <FormFieldsSkeleton fields={1} />
      </div>
      <Skeleton className="ml-auto h-8 w-28" />
    </WorkbenchLoading>
  );
}

export function WorkbenchExportsSkeleton(): ReactElement {
  return (
    <WorkbenchLoading label="正在读取导出账号">
      <FormFieldsSkeleton fields={1} />
      <Skeleton className="h-4 w-24" />
      <ChoiceSkeleton />
      <div
        data-slot="export-account-options"
        className="grid min-w-0 gap-2 rounded-md border p-3 sm:grid-cols-2"
      >
        {[0, 1, 2, 3].map((item) => (
          <ChoiceSkeleton key={item} />
        ))}
      </div>
      <Skeleton className="h-8 w-28" />
    </WorkbenchLoading>
  );
}

export function WorkbenchSecurityBatchSkeleton(): ReactElement {
  return (
    <WorkbenchLoading label="正在读取批量安全设置账号">
      <div className="grid gap-1.5">
        <Skeleton className="h-4 w-24" />
        <div className="py-2">
          <ChoiceSkeleton />
        </div>
        <div
          data-slot="security-account-options"
          className="grid min-w-0 gap-2 rounded-md border p-3 sm:grid-cols-2"
        >
          {[0, 1, 2, 3].map((item) => (
            <ChoiceSkeleton key={item} />
          ))}
        </div>
      </div>
      <FormFieldsSkeleton fields={1} />
      <Skeleton className="ml-auto h-8 w-36" />
    </WorkbenchLoading>
  );
}
