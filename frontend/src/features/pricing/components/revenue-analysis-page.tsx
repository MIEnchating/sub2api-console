import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { toast } from "sonner";

import { api, type RevenueReport, type RevenueRow, type Task } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { cn } from "@/lib/utils";

type RevenueAnalysisView = "details" | "summary" | "issues";

export function defaultRevenueDate(now = new Date()) {
  const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
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
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const page = Math.min(currentPage, totalPages);
  const visibleRows = rows.slice((page - 1) * pageSize, page * pageSize);

  useEffect(() => {
    if (currentPage > totalPages) setCurrentPage(totalPages);
  }, [currentPage, totalPages]);

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border">
      <div className="min-h-0 flex-1 overflow-auto">
        <Table className="min-w-[82rem]">
          <TableHeader>
            <TableRow>
              <TableHead className="w-48">账号</TableHead>
              <TableHead className="w-32">分组</TableHead>
              <TableHead className="w-40">上游 Key</TableHead>
              <TableHead className="w-28 text-right">账号计费</TableHead>
              <TableHead className="w-28 text-right">实际扣费</TableHead>
              <TableHead className="w-32 text-right">上游实际消费</TableHead>
              <TableHead className="w-20 text-right">比例</TableHead>
              <TableHead className="w-28 text-right">上游消费</TableHead>
              <TableHead className="w-32">差额</TableHead>
              <TableHead className="w-28">核对分类</TableHead>
              <TableHead className="w-28 text-right">营收</TableHead>
              <TableHead className="min-w-56">备注</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visibleRows.map((row) => (
              <TableRow key={row.account_id}>
                <TableCell>
                  <span className="font-medium">
                    {row.account_name || `账号 ${row.account_id}`}
                  </span>
                  <span className="text-muted-foreground ml-2">#{row.account_id}</span>
                </TableCell>
                <TableCell>{row.local_group || "未分组"}</TableCell>
                <TableCell>{row.upstream_key_name || "-"}</TableCell>
                <TableCell className="text-right tabular-nums">{money(row.account_cost)}</TableCell>
                <TableCell className="text-right tabular-nums">{money(row.actual_cost)}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {money(row.upstream_raw_cost)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {ratio(row.recharge_rate)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {money(row.upstream_cost)}
                </TableCell>
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
      </div>
      <DataTablePagination
        currentPage={page}
        totalPages={totalPages}
        totalItems={rows.length}
        pageSize={pageSize}
        pageSizes={[10, 20, 50, 100]}
        onPageChange={setCurrentPage}
        onPageSizeChange={(value) => {
          setPageSize(value);
          setCurrentPage(1);
        }}
      />
    </div>
  );
}

function RevenueSummaryTable({ report }: { report: RevenueReport }) {
  return (
    <div className="min-h-0 flex-1 overflow-auto rounded-md border">
      <Table className="min-w-[64rem]">
        <TableHeader>
          <TableRow>
            <TableHead>分组</TableHead>
            <TableHead className="text-right">精确核对账号数</TableHead>
            <TableHead className="text-right">账号计费</TableHead>
            <TableHead className="text-right">实际扣费</TableHead>
            <TableHead className="text-right">上游实际消费</TableHead>
            <TableHead className="text-right">上游消费</TableHead>
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
    </div>
  );
}

export function RevenueAnalysisPage() {
  const queryClient = useQueryClient();
  const [date, setDate] = useState(defaultRevenueDate);
  const [view, setView] = useState<RevenueAnalysisView>("details");
  const [taskID, setTaskID] = useState<string | null>(null);
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
  const report = useMemo(() => revenueReportFromTask(task.data), [task.data]);
  const running = calculate.isPending || (Boolean(taskID) && !taskStopsPolling(task.data));

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
            <Input
              type="date"
              value={date}
              max={defaultRevenueDate()}
              aria-label="核算日期"
              onChange={(event) => setDate(event.target.value)}
              className="w-36"
            />
            <Button onClick={() => calculate.mutate(date)} disabled={running || !date}>
              <Play /> {running ? "核算中" : "开始分析"}
            </Button>
          </PageActions>
        }
      />
      {task.error ? <QueryErrorToast error={task.error} fallback="收益核算任务读取失败" /> : null}
      <div
        className="flex h-full min-h-0 flex-col gap-3 overflow-hidden"
        data-testid="revenue-analysis-page"
      >
        {report ? (
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-b pb-3">
            <Badge variant="outline">精确核对 {report.comparable}</Badge>
            <Badge variant={report.abnormal > 0 ? "destructive" : "outline"}>
              计费异常 {report.abnormal}
            </Badge>
            <Badge variant={report.unavailable > 0 ? "warning" : "outline"}>
              无法核对 {report.unavailable}
            </Badge>
          </div>
        ) : null}

        {running ? (
          <div className="flex min-h-0 flex-col justify-center gap-3 px-1">
            <Progress value={task.data?.progress ?? 5} />
            <p className="text-muted-foreground text-center text-sm">
              {task.data?.message || "正在准备收益分析"}
            </p>
          </div>
        ) : report ? (
          <div className="flex min-h-0 flex-col gap-3 pt-3">
            <div className="bg-muted/40 inline-flex w-fit items-center rounded-md border p-1">
              {(
                [
                  ["details", `账号明细 ${report.rows.length}`],
                  ["summary", "金额统计"],
                  ["issues", `上游读取问题 ${report.issues.length}`],
                ] as const
              ).map(([id, label]) => (
                <Button
                  key={id}
                  size="sm"
                  variant={view === id ? "secondary" : "ghost"}
                  className={cn("h-7", view === id && "bg-background shadow-xs")}
                  onClick={() => setView(id)}
                >
                  {label}
                </Button>
              ))}
            </div>
            {view === "details" ? <RevenueDetails rows={report.rows} /> : null}
            {view === "summary" ? <RevenueSummaryTable report={report} /> : null}
            {view === "issues" ? (
              <div className="min-h-0 flex-1 overflow-auto rounded-md border">
                <Table>
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
              </div>
            ) : null}
          </div>
        ) : task.data?.status === "failed" ? (
          <div className="flex min-h-0 items-center justify-center text-sm text-destructive">
            {task.data.message}
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
