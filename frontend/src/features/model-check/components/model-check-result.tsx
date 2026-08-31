import { AlertTriangle, CheckCircle2, CircleHelp, LoaderCircle, XCircle } from "lucide-react";
import { useEffect, useState } from "react";

import type { Task } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type ResultRecord = Record<string, unknown>;

const verdictLabels: Record<string, string> = {
  MATCH: "匹配",
  GROUP_MATCH: "兼容组匹配",
  MISMATCH: "不匹配",
  INCONCLUSIVE: "无法判定",
  SOL_CONSISTENT: "符合 Sol 行为",
  LUNA_LIKE: "更接近 Luna",
  TERRA_LIKE: "更接近 Terra",
  ERROR: "请求失败",
};

function objectValue(value: unknown): ResultRecord {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as ResultRecord)
    : {};
}

function numberValue(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function textValue(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function resultRows(value: unknown): ResultRecord[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is ResultRecord =>
          item !== null && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

function percent(value: number | null): string {
  return value === null ? "-" : `${value.toFixed(1)}%`;
}

function verdictBadge(verdict: string) {
  if (["MATCH", "GROUP_MATCH", "SOL_CONSISTENT"].includes(verdict)) {
    return (
      <Badge variant="secondary">
        <CheckCircle2 aria-hidden="true" />
        {verdictLabels[verdict]}
      </Badge>
    );
  }
  if (["MISMATCH", "LUNA_LIKE", "TERRA_LIKE", "ERROR"].includes(verdict)) {
    return (
      <Badge variant="destructive">
        <XCircle aria-hidden="true" />
        {verdictLabels[verdict]}
      </Badge>
    );
  }
  return (
    <Badge variant="warning">
      <AlertTriangle aria-hidden="true" />
      {verdictLabels[verdict] ?? verdict}
    </Badge>
  );
}

function requestFailed(result: ResultRecord): boolean {
  const requests = objectValue(result.requests);
  const successful = numberValue(requests.successful);
  const total = numberValue(requests.total);
  return textValue(result.verdict) === "ERROR" || (successful === 0 && total !== null && total > 0);
}

function displayVerdict(result: ResultRecord): string {
  return requestFailed(result) ? "ERROR" : (textValue(result.verdict) ?? "INCONCLUSIVE");
}

function similarity(result: ResultRecord): number | null {
  if (requestFailed(result)) return null;
  if (textValue(result.checker) === "sol") {
    return numberValue(objectValue(result.similarity_percent).sol);
  }
  return numberValue(result.identity_match_percent);
}

function coverage(result: ResultRecord): number | null {
  if (requestFailed(result)) return null;
  if (textValue(result.checker) === "sol") {
    return numberValue(objectValue(result.coverage).percent);
  }
  return numberValue(result.coverage_percent);
}

function resultSummary(rows: ResultRecord[]): ResultRecord {
  const summary: ResultRecord = {};
  for (const row of rows) {
    const verdict = displayVerdict(row);
    summary[verdict] = (numberValue(summary[verdict]) ?? 0) + 1;
  }
  return summary;
}

function summaryBadges(summary: ResultRecord) {
  return Object.entries(summary)
    .filter((entry): entry is [string, number] => typeof entry[1] === "number" && entry[1] > 0)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([verdict, count]) => {
      const variant = verdict === "ERROR" ? "destructive" : "outline";
      return (
        <Badge key={verdict} variant={variant}>
          {verdictLabels[verdict] ?? verdict} {count}
        </Badge>
      );
    });
}

function Metric(props: { label: string; value: number | null }) {
  return (
    <div className="grid min-w-24 gap-1">
      <div className="flex items-center justify-between gap-2 text-xs">
        <span className="text-muted-foreground">{props.label}</span>
        <span className="font-medium tabular-nums">{percent(props.value)}</span>
      </div>
      <Progress value={props.value ?? 0} aria-label={`${props.label} ${percent(props.value)}`} />
    </div>
  );
}

function ResultMetrics(props: { result: ResultRecord }) {
  return (
    <div className="grid gap-2 sm:grid-cols-2 md:grid-cols-1 2xl:grid-cols-2">
      <Metric label="相似度" value={similarity(props.result)} />
      <Metric label="覆盖率" value={coverage(props.result)} />
    </div>
  );
}

export function ModelCheckResult(props: { task: Task }) {
  const rows = resultRows(props.task.result.tests);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const pageRows = rows.slice((page - 1) * pageSize, page * pageSize);

  useEffect(() => {
    setPage(1);
  }, [props.task.id]);

  useEffect(() => {
    setPage((current) => Math.min(current, totalPages));
  }, [totalPages]);

  if (["queued", "running", "waiting_input"].includes(props.task.status)) {
    return (
      <div className="grid h-full place-items-center" aria-live="polite">
        <div className="w-full max-w-lg rounded-md border p-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="flex items-center gap-2 font-medium">
                <LoaderCircle className="text-primary size-4 animate-spin" aria-hidden="true" />
                正在检测
              </p>
              <p className="text-muted-foreground mt-1 text-sm">{props.task.message}</p>
            </div>
            <strong className="text-primary text-lg tabular-nums">{props.task.progress}%</strong>
          </div>
          <div className="mt-4">
            <Progress value={props.task.progress} aria-label="模型检测进度" />
          </div>
        </div>
      </div>
    );
  }
  if (props.task.status === "failed" || props.task.status === "cancelled") {
    return (
      <div className="grid h-full place-items-center">
        <div className="border-destructive/40 bg-destructive/5 w-full max-w-lg rounded-md border p-5">
          <p className="flex items-center gap-2 font-medium">
            <CircleHelp className="text-destructive size-4" aria-hidden="true" />
            检测失败
          </p>
          <p className="text-destructive mt-2 text-sm break-words">
            {textValue(props.task.result.error) ?? props.task.message}
          </p>
        </div>
      </div>
    );
  }

  const summary = resultSummary(rows);
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col" data-testid="model-check-result">
      <div className="flex shrink-0 flex-col gap-2 pb-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="flex items-center gap-2 font-medium">
            <CheckCircle2 className="text-primary size-4" aria-hidden="true" />
            检测完成
          </p>
          <p className="text-muted-foreground mt-0.5 text-xs tabular-nums">
            {rows.length} 个账号模型组合
          </p>
        </div>
        <div className="flex max-w-full flex-wrap gap-1.5 sm:justify-end">
          {summaryBadges(summary)}
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-hidden rounded-md border">
        <Table
          containerClassName="hidden h-full overflow-auto md:block"
          className="min-w-[840px]"
          data-testid="model-check-result-desktop-table"
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-[17%]">账号</TableHead>
              <TableHead className="w-[20%]">模型</TableHead>
              <TableHead className="w-[15%]">结论</TableHead>
              <TableHead className="w-[22%]">行为指标</TableHead>
              <TableHead className="w-[10%]">成功请求</TableHead>
              <TableHead>详情</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground h-24 text-center">
                  任务未返回检测明细
                </TableCell>
              </TableRow>
            ) : null}
            {pageRows.map((result, index) => {
              const verdict = displayVerdict(result);
              const requests = objectValue(result.requests);
              const error =
                textValue(result.error) ??
                (verdict === "ERROR" ? "所有检测请求均失败，请检查账号、模型和上游连接" : null);
              const responseModels = stringArray(result.response_models);
              const detail = error ?? (responseModels.join("、") || "未返回模型");
              return (
                <TableRow
                  key={`${textValue(result.account_id) ?? index}-${textValue(result.claimed_model) ?? index}`}
                  className={
                    verdict === "ERROR" ? "bg-destructive/5 hover:bg-destructive/10" : undefined
                  }
                >
                  <TableCell>
                    <span className="block truncate font-medium">
                      {textValue(result.account_name) ?? "-"}
                    </span>
                    <span className="text-muted-foreground block truncate text-xs">
                      ID {textValue(result.account_id) ?? "-"}
                    </span>
                  </TableCell>
                  <TableCell className="font-medium">
                    {textValue(result.claimed_model) ?? "-"}
                  </TableCell>
                  <TableCell tooltipContent={error ?? verdictLabels[verdict] ?? verdict}>
                    {verdictBadge(verdict)}
                  </TableCell>
                  <TableCell overflowTooltip={false}>
                    <ResultMetrics result={result} />
                  </TableCell>
                  <TableCell
                    className={verdict === "ERROR" ? "text-destructive font-medium" : undefined}
                  >
                    {numberValue(requests.successful) ?? 0}/{numberValue(requests.total) ?? 0}
                  </TableCell>
                  <TableCell tooltipContent={detail} className="max-w-72">
                    <span
                      className={
                        verdict === "ERROR"
                          ? "text-destructive line-clamp-2 whitespace-normal"
                          : "line-clamp-2 whitespace-normal"
                      }
                    >
                      {detail}
                    </span>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
        <div
          className="h-full divide-y overflow-auto md:hidden"
          data-testid="model-check-result-mobile-list"
        >
          {rows.length === 0 ? (
            <div className="text-muted-foreground p-6 text-center text-sm">任务未返回检测明细</div>
          ) : null}
          {pageRows.map((result, index) => {
            const verdict = displayVerdict(result);
            const requests = objectValue(result.requests);
            const error =
              textValue(result.error) ??
              (verdict === "ERROR" ? "所有检测请求均失败，请检查账号、模型和上游连接" : null);
            const responseModels = stringArray(result.response_models);
            const detail = error ?? (responseModels.join("、") || "未返回模型");
            return (
              <article
                key={`${textValue(result.account_id) ?? index}-${textValue(result.claimed_model) ?? index}`}
                className={verdict === "ERROR" ? "bg-destructive/5 p-3" : "p-3"}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{textValue(result.account_name) ?? "-"}</p>
                    <p className="text-muted-foreground truncate text-xs tabular-nums">
                      ID {textValue(result.account_id) ?? "-"}
                    </p>
                  </div>
                  {verdictBadge(verdict)}
                </div>
                <p className="mt-2 truncate text-sm font-medium">
                  {textValue(result.claimed_model) ?? "-"}
                </p>
                <div className="mt-3">
                  <ResultMetrics result={result} />
                </div>
                <div className="mt-3 flex items-center justify-between gap-3 border-t pt-2 text-xs">
                  <span className="text-muted-foreground">成功请求</span>
                  <strong
                    className={
                      verdict === "ERROR" ? "text-destructive tabular-nums" : "tabular-nums"
                    }
                  >
                    {numberValue(requests.successful) ?? 0}/{numberValue(requests.total) ?? 0}
                  </strong>
                </div>
                <p
                  className={
                    verdict === "ERROR"
                      ? "text-destructive mt-2 break-words text-xs"
                      : "text-muted-foreground mt-2 break-words text-xs"
                  }
                >
                  {detail}
                </p>
              </article>
            );
          })}
        </div>
      </div>
      <div className="shrink-0">
        <DataTablePagination
          currentPage={page}
          totalPages={totalPages}
          totalItems={rows.length}
          pageSize={pageSize}
          pageSizes={[10, 20, 50]}
          onPageChange={setPage}
          onPageSizeChange={(value) => {
            setPageSize(value);
            setPage(1);
          }}
        />
      </div>
    </div>
  );
}
