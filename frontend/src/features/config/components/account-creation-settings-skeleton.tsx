import type { ReactElement } from "react";
import { FormFieldsSkeleton } from "@/components/form-fields-skeleton";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { SettingsFooter } from "./settings-footer";
import { settingsLayout } from "./settings-layout";
import { cn } from "@/lib/utils";

export function AccountCreationSettingsSkeleton(): ReactElement {
  return (
    <Card
      size="sm"
      className="h-full min-h-0 min-w-0"
      role="status"
      aria-busy="true"
      aria-label="正在读取账号设置"
    >
      <CardHeader className="shrink-0">
        <CardTitle>账号设置</CardTitle>
        <CardDescription>设置新账号参数，为不同分组保存各自配置</CardDescription>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col group-data-[size=sm]/card:p-0">
        <div
          aria-hidden="true"
          className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-3 py-3"
        >
          <div className="grid min-w-0 gap-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </div>
          <Skeleton className="h-8 w-full sm:w-40" />
        </div>
        <Skeleton className="mx-3 my-3 h-8 shrink-0" />
        <div
          data-slot="settings-scroll"
          className={cn(
            settingsLayout.accountPolicyFields,
            "min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3",
          )}
        >
          <div className="grid min-w-0 gap-1.5" aria-hidden="true">
            <Skeleton className="h-4 w-20" />
            <Skeleton data-slot="account-models-skeleton" className="h-44 w-full" />
          </div>
          <div className="grid min-w-0 content-start gap-4" aria-hidden="true">
            <FormFieldsSkeleton fields={3} className="sm:grid-cols-3" />
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <Skeleton className="h-4 w-20" />
              <Skeleton className="h-4 w-7 rounded-full" />
            </div>
          </div>
        </div>
        <SettingsFooter>
          <Skeleton className="h-8 w-28" />
        </SettingsFooter>
      </CardContent>
    </Card>
  );
}
