import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { toast } from "sonner";

import { api, type RevenueReport, type RevenueRow, type Task } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { DatePicker } from "@/components/date-picker";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { useClientPagination } from "@/hooks/use-client-pagination";

type RevenueAnalysisView = "details" | "summary" | "issues";

export function defaultRevenueDate(now = new Date()) {
  const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
}

export function revenueDateValue(value: string): Date | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return undefined;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  if (
    date.getFullYear() !== Number(match[1]) ||
    date.getMonth() !== Number(match[2]) - 1 ||
    date.getDate() !== Number(match[3])
  ) {
    return undefined;
  }
  return date;
}

function revenueDateText(value: Date | undefined): string {
  if (!value) return "";
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${value.getFullYear()}-${month}-${day}`;
}

export function revenueReportFromTask(task: Task | undefined): RevenueReport | null {
  if (!task || task.status !== "succeeded" || task.operation !== "revenue-calculation") return null;
  const value = task.result;
  if (
    typeof value.report_date !== "string" ||
    typeof value.timezone !== "string" ||
    !Array.isArray(value.rows) ||
    !Array.isArray(value.summaries) ||
    !Array.isArray(value.issues)
  ) {
    return null;
  }
  return value as RevenueReport;
}

function money(value: number | null) {
  if (value === null || !Number.isFinite(value)) return "-";
  return `${value < 0 ? "-" : ""}$${Math.abs(value).toFixed(2)}`;
}

function difference(value: number | null) {
  if (value === null || !Number.isFinite(value)) return "-";
  if (Math.abs(value) < 0.005) return "$0.00";
  return `${value < 0 ? "亏损" : "盈余"} $${Math.abs(value).toFixed(2)}`;
}

function ratio(value: number | null) {
  return value === null || !Number.isFinite(value) ? "-" : `÷${value}`;
}

function categoryBadge(category: RevenueRow["category"]) {
  if (category === "计费异常") return <Badge variant="destructive">计费异常</Badge>;
  if (category === "正常") return <Badge variant="outline">正常</Badge>;
  return <Badge variant="warning">无法核对</Badge>;
}

function RevenueDetails({ rows }: { rows: RevenueRow[] }) {
  const pagination = useClientPagination(rows);

  return (
    <DataTablePanel className="flex-1">
      <Table
        className="min-w-[100rem] table-fixed"
        containerClassName="min-h-0 flex-1 overflow-auto"
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-56">账号</TableHead>
            <TableHead className="w-40">分组</TableHead>
            <TableHead className="w-48">上游 Key</TableHead>
            <TableHead className="w-28 text-right">账号计费</TableHead>
            <TableHead className="w-28 text-right">实际扣费</TableHead>
            <TableHead className="w-32 text-right">上游原始消费</TableHead>
            <TableHead className="w-20 text-right">充值比例</TableHead>
            <TableHead className="w-28 text-right">换算后成本</TableHead>
            <TableHead className="w-32">差额</TableHead>
            <TableHead className="w-28">核对分类</TableHead>
            <TableHead className="w-28 text-right">营收</TableHead>
            <TableHead className="min-w-56">备注</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pagination.visibleItems.map((row) => (
            <TableRow key={row.account_id}>
              <TableCell
                className="overflow-hidden"
                tooltipContent={`${row.account_name || `账号 ${row.account_id}`} · #${row.account_id}`}
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 truncate font-medium">
                    {row.account_name || `账号 ${row.account_id}`}
                  </span>
                  <span className="text-muted-foreground shrink-0">#{row.account_id}</span>
                </div>
              </TableCell>
              <TableCell className="overflow-hidden" tooltipContent={row.local_group || "未分组"}>
                {row.local_group || "未分组"}
              </TableCell>
              <TableCell className="overflow-hidden" tooltipContent={row.upstream_key_name || "-"}>
                {row.upstream_key_name || "-"}
              </TableCell>
              <TableCell className="text-right tabular-nums">{money(row.account_cost)}</TableCell>
              <TableCell className="text-right tabular-nums">{money(row.actual_cost)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {money(row.upstream_raw_cost)}
              </TableCell>
              <TableCell className="text-right tabular-nums">{ratio(row.recharge_rate)}</TableCell>
              <TableCell className="text-right tabular-nums">{money(row.upstream_cost)}</TableCell>
              <TableCell className="tabular-nums">{difference(row.difference)}</TableCell>
              <TableCell>{categoryBadge(row.category)}</TableCell>
              <TableCell className="text-right font-medium tabular-nums">
                {money(row.revenue)}
              </TableCell>
              <TableCell className="text-muted-foreground text-xs leading-5">
                {row.note || "-"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <DataTablePagination
        currentPage={pagination.currentPage}
        totalPages={pagination.totalPages}
        totalItems={rows.length}
        pageSize={pagination.pageSize}
        pageSizes={[10, 20, 50, 100]}
        onPageChange={pagination.setCurrentPage}
        onPageSizeChange={pagination.setPageSize}
      />
    </DataTablePanel>
  );
}

function RevenueSummaryTable({ report }: { report: RevenueReport }) {
  return (
    <DataTablePanel className="flex-1">
      <Table className="min-w-[64rem]" containerClassName="min-h-0 flex-1 overflow-auto">
        <TableHeader>
          <TableRow>
            <TableHead>分组</TableHead>
            <TableHead className="text-right">精确核对账号数</TableHead>
            <TableHead className="text-right">账号计费</TableHead>
            <TableHead className="text-right">实际扣费</TableHead>
            <TableHead className="text-right">上游原始消费</TableHead>
            <TableHead className="text-right">换算后成本</TableHead>
            <TableHead>差额</TableHead>
            <TableHead className="text-right">营收</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {report.summaries.map((row) => (
            <TableRow
              key={row.group}
              className={row.group === "合计" ? "bg-muted/35 font-medium" : undefined}
            >
              <TableCell>{row.group}</TableCell>
              <TableCell className="text-right tabular-nums">{row.accounts}</TableCell>
              <TableCell className="text-right tabular-nums">{money(row.account_cost)}</TableCell>
              <TableCell className="text-right tabular-nums">{money(row.actual_cost)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {money(row.upstream_raw_cost)}
              </TableCell>
              <TableCell className="text-right tabular-nums">{money(row.upstream_cost)}</TableCell>
              <TableCell className="tabular-nums">{difference(row.difference)}</TableCell>
              <TableCell className="text-right tabular-nums">{money(row.revenue)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataTablePanel>
  );
}

export function RevenueCalculationProgress(props: { progress: number }) {
  const progress = Math.min(100, Math.max(0, Math.round(props.progress)));
  return (
    <div
      className="flex min-h-0 flex-1 items-center justify-center"
      aria-label="收益分析进度"
      aria-live="polite"
    >
      <div className="w-full max-w-md space-y-3 px-4">
        <div className="flex items-center justify-between gap-3">
          <span className="text-muted-foreground text-sm">正在分析</span>
          <strong className="text-foreground text-sm font-semibold tabular-nums">
            {progress}%
          </strong>
        </div>
        <Progress value={progress} />
      </div>
    </div>
  );
}

export function RevenueAnalysisPage() {
  const queryClient = useQueryClient();
  const [date, setDate] = useState(defaultRevenueDate);
  const [view, setView] = useState<RevenueAnalysisView>("details");
  const [taskID, setTaskID] = useState<string | null>(null);
  const latest = useQuery({
    queryKey: ["pricing-revenue-latest"],
    queryFn: api.latestRevenue,
  });
  const task = useQuery({
    queryKey: ["task", taskID],
    queryFn: () => api.task(taskID!),
    enabled: Boolean(taskID),
    refetchInterval: taskPollInterval,
  });
  const calculate = useMutation({
    mutationFn: api.calculateRevenue,
    onSuccess: (queued) => {
      setTaskID(queued.id);
      queryClient.setQueryData(["task", queued.id], queued);
      toast.success("收益核算已开始");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "收益核算启动失败"),
  });
  const currentReport = useMemo(() => revenueReportFromTask(task.data), [task.data]);
  const latestReport = useMemo(
    () => revenueReportFromTask(latest.data ?? undefined),
    [latest.data],
  );
  const report = currentReport ?? latestReport;
  const running = calculate.isPending || (Boolean(taskID) && !taskStopsPolling(task.data));

  useEffect(() => {
    if (!currentReport || !task.data) return;
    queryClient.setQueryData(["pricing-revenue-latest"], task.data);
  }, [currentReport, queryClient, task.data]);

  useEffect(() => {
    if (!taskID && latestReport) setDate(latestReport.report_date);
  }, [latestReport, taskID]);

  useEffect(() => {
    if (report) setView("details");
  }, [report]);

  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="OPERATIONS / REVENUE"
        title="收益分析"
        description={
          report
            ? `${report.report_date} · ${report.timezone} · 仅汇总稳定 Key/Token 精确归因结果`
            : "按完整自然日核对账号计费、实际扣费和上游消费。"
        }
        action={
          <PageActions>
            <DatePicker
              selected={revenueDateValue(date)}
              toDate={revenueDateValue(defaultRevenueDate())}
              label="核算日期"
              clearable={false}
              onSelect={(value) => setDate(revenueDateText(value))}
              className="w-44"
            />
            <Button onClick={() => calculate.mutate(date)} disabled={running || !date}>
              <Play /> {running ? "核算中" : "开始分析"}
            </Button>
          </PageActions>
        }
      />
      {task.error ? <QueryErrorToast error={task.error} fallback="收益核算任务读取失败" /> : null}
      {latest.error ? (
        <QueryErrorToast error={latest.error} fallback="最近收益分析读取失败" />
      ) : null}
      <div
        className="flex h-full min-h-0 flex-col gap-3 overflow-hidden"
        data-testid="revenue-analysis-page"
      >
        {running ? (
          <RevenueCalculationProgress progress={task.data?.progress ?? 0} />
        ) : task.data?.status === "failed" ? (
          <div className="flex min-h-0 items-center justify-center text-sm text-destructive">
            {task.data.message}
          </div>
        ) : report ? (
          <div className="flex min-h-0 flex-1 flex-col gap-3">
            <SegmentedControl aria-label="收益分析视图">
              {(
                [
                  ["details", "账号明细"],
                  ["summary", "金额统计"],
                  ["issues", "上游读取问题"],
                ] as const
              ).map(([id, label]) => (
                <SegmentedControlItem key={id} selected={view === id} onClick={() => setView(id)}>
                  {label}
                </SegmentedControlItem>
              ))}
            </SegmentedControl>
            {view === "details" ? <RevenueDetails rows={report.rows} /> : null}
            {view === "summary" ? <RevenueSummaryTable report={report} /> : null}
            {view === "issues" ? (
              <DataTablePanel className="flex-1">
                <Table containerClassName="min-h-0 flex-1 overflow-auto">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-64">Host</TableHead>
                      <TableHead>原因</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {report.issues.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={2} className="text-muted-foreground h-24 text-center">
                          没有上游读取问题
                        </TableCell>
                      </TableRow>
                    ) : (
                      report.issues.map((issue) => (
                        <TableRow key={`${issue.host}:${issue.reason}`}>
                          <TableCell className="font-medium">{issue.host}</TableCell>
                          <TableCell>{issue.reason}</TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </DataTablePanel>
            ) : null}
          </div>
        ) : latest.isLoading ? (
          <div className="flex min-h-0 flex-1 items-center justify-center">
            <span className="text-muted-foreground text-sm">正在读取最近一次分析</span>
          </div>
        ) : (
          <div className="text-muted-foreground flex min-h-0 items-center justify-center text-sm">
            尚未生成核算结果
          </div>
        )}
      </div>
    </PageLayout>
  );
}
