import { useState, type ReactElement } from "react";
import { CheckCircle2, CircleSlash, XCircle } from "lucide-react";
import type { Task } from "@/api";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { verificationIdentityExplanation } from "../constants";
import {
  verificationNumber as number,
  verificationRecord as record,
  verificationRows as rows,
  verificationText as text,
  verificationProbeGroups,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationProgressGroup } from "./verification-progress-group";
import { VerificationFingerprintDetails } from "./verification-fingerprint-details";
import {
  VerificationCapabilityStatus,
  VerificationIdentityStatus,
  VerificationIdentitySummary,
} from "./verification-status";

function InlineResult(props: { result: VerificationRecord; multiple: boolean }): ReactElement {
  const [view, setView] = useState("probes");
  const [onlyErrors, setOnlyErrors] = useState(false);
  const result = props.result;
  const probes = rows(result.probes);
  const errors = probes.filter((probe) => probe.status === "failed" || probe.status === "error");
  const knowledge = record(record(result.public_reference).knowledge);
  const groups = verificationProbeGroups(onlyErrors ? errors : probes);
  return (
    <article className="min-w-0 space-y-4 py-4 first:pt-0 last:pb-0">
      {props.multiple ? (
        <h3 className="font-medium wrap-anywhere">
          {text(result.account_name)} · {text(result.claimed_model)}
        </h3>
      ) : null}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <VerificationCapabilityStatus status={text(record(result.capability).status)} />
          <VerificationIdentityStatus result={result} />
        </div>
        <VerificationIdentitySummary result={result} prominent />
        {text(result.error) ? (
          <p className="text-sm text-destructive wrap-anywhere">{text(result.error)}</p>
        ) : null}
        {record(result.capability).status !== "passed" &&
        text(record(result.capability).summary) ? (
          <p className="text-xs text-muted-foreground wrap-anywhere">
            {text(record(result.capability).summary)}
          </p>
        ) : null}
        <dl aria-label="检测统计" className="grid grid-cols-3 divide-x border-y py-3 text-xs">
          <div className="space-y-1 pr-3">
            <dt className="text-muted-foreground">能力题通过</dt>
            <dd className="font-medium tabular-nums">
              {number(result.passed) ?? "—"} / {number(result.capability_total) ?? "—"}
            </dd>
          </div>
          <div className="space-y-1 px-3">
            <dt className="text-muted-foreground">请求成功</dt>
            <dd className="font-medium tabular-nums">
              {number(record(result.requests).successful) ??
                probes.filter((probe) =>
                  ["passed", "failed", "observed"].includes(text(probe.status)),
                ).length}{" "}
              / {number(result.total) ?? probes.length}
            </dd>
          </div>
          <div className="space-y-1 pl-3">
            <dt className="text-muted-foreground">知识题有效</dt>
            <dd className="font-medium tabular-nums">
              {number(knowledge.valid) ?? "—"} / {number(knowledge.total) ?? "—"}
            </dd>
          </div>
        </dl>
      </div>
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-b pb-2">
        <div role="group" aria-label="结果视图" className="flex min-w-0 gap-1">
          <Button
            variant={view === "probes" ? "secondary" : "ghost"}
            aria-pressed={view === "probes"}
            onClick={() => setView("probes")}
          >
            逐项检测结果
          </Button>
          <Button
            variant={view === "evidence" ? "secondary" : "ghost"}
            aria-pressed={view === "evidence"}
            onClick={() => setView("evidence")}
          >
            来源与指纹
          </Button>
        </div>
        {view === "probes" && errors.length > 0 ? (
          <Button
            variant="ghost"
            className="text-destructive"
            aria-pressed={onlyErrors}
            onClick={() => setOnlyErrors(!onlyErrors)}
          >
            仅看异常 {errors.length}
          </Button>
        ) : null}
      </div>
      {view === "probes" ? (
        <div className="min-w-0 space-y-2">
          {groups.map((group) => (
            <VerificationProgressGroup
              key={`${onlyErrors}-${group.label}`}
              label={group.label}
              probes={group.probes}
              timeout={null}
              showFullDetails
              defaultExpanded={group.probes.some(
                (probe) => probe.status === "failed" || probe.status === "error",
              )}
            />
          ))}
          {probes.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">没有逐项记录</p>
          ) : null}
        </div>
      ) : (
        <section aria-label="来源与对照证据" className="min-w-0 space-y-3">
          <VerificationFingerprintDetails result={result} />
          {rows(result.evidence)
            .filter((item) => !(item.id === "reference" && item.status === "unknown"))
            .map((item, index) => (
              <div key={text(item.id) || index} className="space-y-1 border-b pb-3 last:border-0">
                <h4 className="text-sm font-medium">{text(item.label)}</h4>
                <p className="text-xs leading-relaxed text-muted-foreground wrap-anywhere">
                  {text(item.detail)}
                </p>
              </div>
            ))}
          <details className="text-xs text-muted-foreground">
            <summary className="cursor-pointer">检测范围与结论说明</summary>
            <p className="mt-2 leading-relaxed">{verificationIdentityExplanation}</p>
          </details>
        </section>
      )}
    </article>
  );
}

export function VerificationInlineReport(props: {
  task: Task;
  results: VerificationRecord[];
}): ReactElement {
  const failed = props.task.status === "failed";
  const cancelled = props.task.status === "cancelled";
  let heading = "检测完成";
  let Icon = CheckCircle2;
  if (failed) {
    heading = "检测失败";
    Icon = XCircle;
  } else if (cancelled) {
    heading = "检测已取消";
    Icon = CircleSlash;
  }
  return (
    <div className="min-w-0 space-y-4" data-testid="model-check-result">
      <header className="space-y-2">
        <p
          className={cn(
            "flex items-center gap-2 text-sm font-medium",
            failed && "text-destructive",
          )}
        >
          <Icon
            aria-hidden="true"
            className={cn("size-4 shrink-0", !failed && !cancelled && "text-success")}
          />
          {heading} · {props.results.length} 个组合
        </p>
        {failed || cancelled ? (
          <p className="text-xs text-muted-foreground wrap-anywhere">
            {text(props.task.result.error) || props.task.message}
          </p>
        ) : null}
      </header>
      <section aria-label="账号模型检测结果" className="min-w-0 divide-y">
        {props.results.map((result, index) => (
          <InlineResult
            key={`${props.task.id}-${text(result.account_id)}-${text(result.claimed_model)}-${index}`}
            result={result}
            multiple={props.results.length > 1}
          />
        ))}
      </section>
    </div>
  );
}
