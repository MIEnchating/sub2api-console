import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { ContentRetry } from "@/components/content-retry";
import {
  maintenanceKey,
  maintenanceStatusLabels,
  maintenanceStatusVariants,
  maintenanceActionLabels,
  maintenanceReasonLabels,
} from "../constants";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { Badge } from "@/components/ui/badge";
import { RefreshCw, HeartPulse } from "lucide-react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { WorkbenchToolbar, WorkbenchEmptyState } from "./workbench-section";
import { MaintenanceForm } from "./maintenance-form";

export function MaintenancePanel(): ReactElement {
  const [tab, setTab] = useState("results");
  const query = useQuery({
    queryKey: maintenanceKey,
    queryFn: ({ signal }) => api.workbenchMaintenance(signal),
    refetchInterval: (query) => (query.state.data?.running ? 1000 : 30_000),
  });
  if (query.isPending)
    return (
      <div aria-busy="true" aria-label="正在读取维护设置" className="grid gap-4">
        <Skeleton className="ml-auto h-8 w-20" />
        <div className="grid gap-3">
          <Skeleton className="h-24" />
          <Skeleton className="h-64" />
        </div>
      </div>
    );
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  const value = query.data;
  return (
    <section aria-label="自动维护" className="grid min-w-0 gap-4">
      <WorkbenchToolbar
        description="按分组定时检查账号，查看恢复进度与结果。"
        actions={
          <div className="flex flex-wrap gap-2">
            <MaintenanceForm value={value} />
            <Button
              variant="outline"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCw aria-hidden="true" />
              刷新
            </Button>
          </div>
        }
      />
      <section aria-label="维护结果" className="grid min-w-0 gap-4 rounded-lg border bg-card p-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
          <SegmentedControl role="tablist" aria-label="维护信息">
            {[
              { id: "status", label: "运行状态" },
              { id: "results", label: "账号结果" },
            ].map((item) => (
              <SegmentedControlItem
                key={item.id}
                role="tab"
                id={`maintenance-tab-${item.id}`}
                aria-controls={`maintenance-panel-${item.id}`}
                selected={tab === item.id}
                onClick={() => setTab(item.id)}
              >
                {item.label}
              </SegmentedControlItem>
            ))}
          </SegmentedControl>
          <Badge variant={value.enabled ? "success" : "neutral"}>
            {value.enabled ? "定时检查已启用" : "定时检查已关闭"}
          </Badge>
        </div>
        <div
          role="tabpanel"
          id="maintenance-panel-status"
          aria-labelledby="maintenance-tab-status"
          hidden={tab !== "status"}
          className="min-w-0 overflow-x-auto rounded-lg border"
        >
          <Table aria-label="维护运行状态" className="min-w-[540px]">
            <TableHeader>
              <TableRow>
                <TableHead>运行状态</TableHead>
                <TableHead>上次检查</TableHead>
                <TableHead>下次检查</TableHead>
                <TableHead>任务 ID</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell className="whitespace-normal wrap-anywhere" overflowTooltip={false}>
                  <span role="status">
                    {value.running ? "正在维护账号…" : value.message || "尚未运行"}
                  </span>
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {value.last_check_at
                    ? new Date(value.last_check_at).toLocaleString("zh-CN")
                    : "尚未检查"}
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {value.next_check_at
                    ? new Date(value.next_check_at).toLocaleString("zh-CN")
                    : "定时检查未启动"}
                </TableCell>
                <TableCell className="font-mono text-xs">{value.task_id || "—"}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
        <div
          role="tabpanel"
          id="maintenance-panel-results"
          aria-labelledby="maintenance-tab-results"
          hidden={tab !== "results"}
        >
          {value.results.length === 0 && (
            <WorkbenchEmptyState
              icon={HeartPulse}
              title="暂无检查结果"
              description="保存维护设置并执行检查后，这里会展示每个账号的处理结果。"
            />
          )}
          {value.results.length > 0 && (
            <div className="min-w-0 overflow-x-auto rounded-lg border">
              <Table aria-label="本轮账号维护结果" className="min-w-[560px]">
                <TableHeader>
                  <TableRow>
                    <TableHead>账号</TableHead>
                    <TableHead>操作</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>说明</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {value.results.map((row) => (
                    <TableRow key={row.account_id}>
                      <TableCell className="max-w-64 whitespace-normal wrap-anywhere">
                        {row.email || `账号 #${row.account_id}`}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        {maintenanceActionLabels[row.action] || "检查"}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        <Badge variant={maintenanceStatusVariants[row.status] ?? "warning"}>
                          {maintenanceStatusLabels[row.status] || "需要处理"}
                        </Badge>
                      </TableCell>
                      <TableCell className="min-w-48">
                        {maintenanceReasonLabels[row.reason] || "请检查站点账号状态"}
                        {row.next_eligible_at && (
                          <span className="mt-1 block text-xs text-muted-foreground">
                            {new Date(row.next_eligible_at).toLocaleString("zh-CN")} 后再检查
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </section>
    </section>
  );
}
