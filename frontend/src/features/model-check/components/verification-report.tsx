import { CircleHelp, XCircle } from "lucide-react";
import { useEffect, type ReactElement } from "react";
import type { Task } from "@/api";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { verificationIdentityExplanation, verificationBehaviorLabels } from "../constants";
import {
  verificationNumber as number,
  verificationRecord as record,
  verificationRows as rows,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationInlineReport } from "./verification-inline-report";
import { VerificationResult } from "./verification-result";
import {
  VerificationCapabilityStatus,
  VerificationIdentityStatus,
  VerificationIdentitySummary,
} from "./verification-status";

export function VerificationReport(props: {
  task: Task;
  results: VerificationRecord[];
  embedded?: boolean;
}): ReactElement {
  const pagination = useClientPagination(props.results);
  useEffect(() => {
    pagination.setCurrentPage(1);
  }, [pagination.setCurrentPage, props.task.id]);
  if (props.embedded) return <VerificationInlineReport task={props.task} results={props.results} />;
  const passed = props.results.filter(
    (result) => record(result.capability).status === "passed",
  ).length;
  const failed = props.results.filter(
    (result) => record(result.capability).status === "failed",
  ).length;
  let heading = "检测完成";
  if (props.task.status === "cancelled") heading = "检测已取消";
  else if (props.task.status === "failed") heading = "检测失败";
  return (
    <div
      className={cn("flex min-h-0 min-w-0 flex-col gap-3", !props.embedded && "h-full")}
      data-testid="model-check-result"
    >
      <div className="shrink-0 space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="font-medium">
            {heading} · {props.results.length} 个组合
          </p>
          <p className="text-muted-foreground text-xs">{text(props.task.result.profile_version)}</p>
        </div>
        {props.task.status === "failed" ? (
          <p className="text-destructive text-sm wrap-anywhere">
            {text(props.task.result.error) || props.task.message}
          </p>
        ) : null}
        <div className="grid grid-cols-3 divide-x rounded-md border bg-muted/20 text-sm">
          <div className="p-3">
            <p className="text-muted-foreground text-xs">能力通过</p>
            <strong className="text-success text-xl tabular-nums">{passed}</strong>
          </div>
          <div className="p-3">
            <p className="text-muted-foreground text-xs">能力未通过</p>
            <strong className="text-xl tabular-nums">{failed}</strong>
          </div>
          <div className="p-3">
            <p className="text-muted-foreground text-xs">能力待判定</p>
            <strong className="text-xl tabular-nums">
              {props.results.length - passed - failed}
            </strong>
          </div>
        </div>
        <p className="flex items-start gap-2 text-xs text-muted-foreground">
          <CircleHelp className="size-4 shrink-0" aria-hidden="true" />
          {verificationIdentityExplanation}
        </p>
      </div>
      <section
        aria-label="账号模型检测结果"
        className={cn("min-h-0 rounded-md border", !props.embedded && "flex-1 overflow-auto")}
      >
        {pagination.visibleItems.map((result, index) => (
          <article
            key={`${text(result.account_id)}-${text(result.claimed_model)}-${index}`}
            className="grid min-w-0 gap-3 border-b p-3 last:border-b-0 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-center sm:p-4"
          >
            <div className="min-w-0 space-y-1">
              <p className="font-medium wrap-anywhere">
                {text(result.account_name) || "账号"}
                <span className="text-muted-foreground ml-2 text-xs">
                  ID {text(result.account_id) || "—"}
                </span>
              </p>
              <p className="text-sm wrap-anywhere">{text(result.claimed_model) || "模型未记录"}</p>
              <p className="text-muted-foreground text-xs tabular-nums">
                题目通过 {number(result.passed) ?? "—"}/
                {number(result.capability_total) ?? number(result.total) ?? "—"} · 成功请求{" "}
                {number(record(result.requests).successful) ?? "—"}/
                {number(record(result.requests).total) ?? "—"}
              </p>
            </div>
            <div className="min-w-0 space-y-2">
              <div className="flex flex-wrap gap-2">
                <VerificationCapabilityStatus status={text(record(result.capability).status)} />
                <VerificationIdentityStatus result={result} />
              </div>
              <VerificationIdentitySummary result={result} />
              {record(result.behavior_fingerprint).version ? (
                <p className="text-xs text-muted-foreground">
                  行为指纹：
                  {verificationBehaviorLabels[text(record(result.behavior_fingerprint).status)] ??
                    "无法判定"}{" "}
                  · 有效 {number(record(result.behavior_fingerprint).valid) ?? 0}/
                  {number(record(result.behavior_fingerprint).planned) ?? 200}
                </p>
              ) : null}
              {rows(result.evidence).some((item) => item.status === "warning") ? (
                <Badge variant="warning">有待核对证据</Badge>
              ) : null}
              {result.verdict === "ERROR" ? (
                <Badge variant="destructive">
                  <XCircle aria-hidden="true" />
                  请求失败
                </Badge>
              ) : null}
              {text(result.error) ? (
                <p className="text-destructive text-xs wrap-anywhere">{text(result.error)}</p>
              ) : null}
              {record(result.capability).status !== "passed" &&
              text(record(result.capability).summary) ? (
                <p className="text-muted-foreground text-xs wrap-anywhere">
                  {text(record(result.capability).summary)}
                </p>
              ) : null}
            </div>
            <VerificationResult result={result} actionOnly />
          </article>
        ))}
      </section>
      {props.results.length > 20 ? (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={props.results.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 50]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      ) : null}
    </div>
  );
}
