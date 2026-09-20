import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { ContentRetry } from "@/components/content-retry";
import {
  maintenanceKey,
  maintenanceStatusLabels,
  maintenanceActionLabels,
  maintenanceReasonLabels,
} from "../constants";
import { Badge } from "@/components/ui/badge";
import { RefreshCw, HeartPulse } from "lucide-react";
import { WorkbenchToolbar, WorkbenchEmptyState } from "./workbench-section";
import { MaintenanceForm } from "./maintenance-form";

export function MaintenancePanel(): ReactElement {
  const query = useQuery({
    queryKey: maintenanceKey,
    queryFn: api.workbenchMaintenance,
    refetchInterval: (query) => (query.state.data?.running ? 1000 : 30_000),
  });
  if (query.isPending)
    return (
      <div aria-busy="true" aria-label="正在读取维护设置" className="grid gap-4">
        <Skeleton className="ml-auto h-8 w-20" />
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-80" />
          <Skeleton className="h-80" />
        </div>
      </div>
    );
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  const value = query.data;
  return (
    <section aria-label="自动维护" className="grid min-w-0 gap-4">
      <WorkbenchToolbar
        actions={
          <Button
            variant="outline"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" />
            刷新
          </Button>
        }
      />
      <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(20rem,0.85fr)_minmax(0,1.15fr)]">
        <MaintenanceForm value={value} />
        <section aria-label="维护结果" className="grid min-w-0 gap-4 rounded-xl border bg-card p-4">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
            <h3 className="text-sm font-semibold">运行状态与结果</h3>
            <Badge variant="secondary">{value.enabled ? "定时检查已启用" : "定时检查已关闭"}</Badge>
          </div>
          <div className="grid gap-2 rounded-lg bg-muted/30 p-3 text-sm">
            <span role="status" className="wrap-anywhere">
              {value.running ? "正在维护账号…" : value.message || "自动维护尚未配置"}
            </span>
          </div>
          <dl className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="min-w-0 rounded-lg border p-3">
              <dt className="text-xs text-muted-foreground">上次检查</dt>
              <dd className="mt-1 text-sm tabular-nums wrap-anywhere">
                {value.last_check_at
                  ? new Date(value.last_check_at).toLocaleString("zh-CN")
                  : "尚未检查"}
              </dd>
            </div>
            <div className="min-w-0 rounded-lg border p-3">
              <dt className="text-xs text-muted-foreground">下次检查</dt>
              <dd className="mt-1 text-sm tabular-nums wrap-anywhere">
                {value.next_check_at
                  ? new Date(value.next_check_at).toLocaleString("zh-CN")
                  : "定时检查未启动"}
              </dd>
            </div>
          </dl>
          {value.results.length === 0 && (
            <WorkbenchEmptyState
              icon={HeartPulse}
              title="暂无检查结果"
              description="保存维护设置并执行检查后，这里会展示每个账号的处理结果。"
            />
          )}
          {value.results.length > 0 && (
            <div className="min-w-0 overflow-x-auto rounded-lg border">
              <table className="w-full min-w-[560px] text-left text-sm">
                <caption className="sr-only">本轮账号维护结果</caption>
                <thead>
                  <tr className="border-b bg-muted/30">
                    <th className="p-3">账号</th>
                    <th className="p-3">操作</th>
                    <th className="p-3">状态</th>
                    <th className="p-3">说明</th>
                  </tr>
                </thead>
                <tbody>
                  {value.results.map((row) => (
                    <tr key={row.account_id} className="border-b last:border-0">
                      <td className="max-w-64 p-3 wrap-anywhere">
                        {row.email || `账号 #${row.account_id}`}
                      </td>
                      <td className="whitespace-nowrap p-3">
                        {maintenanceActionLabels[row.action] || "检查"}
                      </td>
                      <td className="whitespace-nowrap p-3">
                        {maintenanceStatusLabels[row.status] || "需要处理"}
                      </td>
                      <td className="min-w-48 p-3">
                        {maintenanceReasonLabels[row.reason] || "请检查站点账号状态"}
                        {row.next_eligible_at && (
                          <span className="mt-1 block text-xs text-muted-foreground">
                            {new Date(row.next_eligible_at).toLocaleString("zh-CN")} 后再检查
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </section>
  );
}
