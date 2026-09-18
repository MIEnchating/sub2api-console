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
import { RefreshCw } from "lucide-react";
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
        <Skeleton className="h-48" />
        <Skeleton className="h-32" />
      </div>
    );
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  const value = query.data;
  return (
    <section aria-label="自动维护" className="grid min-w-0 gap-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">自动维护</h2>
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>
          <RefreshCw aria-hidden="true" />
          刷新
        </Button>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        定期检查授权，异常时先刷新；仍有认证错误时，用本次登录会话内有效的账号资料尝试一次重新授权。
      </p>
      <div className="grid min-w-0 items-start gap-4 xl:grid-cols-[22rem_minmax(0,1fr)]">
        <MaintenanceForm value={value} />
        <section aria-label="维护结果" className="grid min-w-0 gap-4 rounded-xl border bg-card p-4">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
            <h3 className="text-sm font-semibold">运行状态与结果</h3>
            <Badge variant="secondary">{value.enabled ? "定时检查已启用" : "定时检查已关闭"}</Badge>
          </div>
          <div className="grid gap-2 rounded-lg bg-muted/30 p-3 text-sm">
            <span role="status">
              {value.running ? "正在维护账号…" : value.message || "自动维护尚未配置"}
            </span>
            <span className="text-muted-foreground">
              {value.next_check_at
                ? `下次检查：${new Date(value.next_check_at).toLocaleString("zh-CN")}`
                : "定时检查未启动"}
            </span>
          </div>
          {value.results.length === 0 && (
            <p className="rounded-lg border border-dashed px-3 py-10 text-center text-sm text-muted-foreground">
              暂无检查结果
            </p>
          )}
          {value.results.length > 0 && (
            <div className="min-w-0 overflow-x-auto rounded-lg border">
              <table className="w-full text-left text-sm">
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
                      <td className="max-w-64 break-all p-3">
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
