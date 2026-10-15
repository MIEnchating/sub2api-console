import type { ReactElement } from "react";
import { Badge } from "@/components/ui/badge";
import {
  verificationNumber as number,
  verificationRows as rows,
  verificationText as text,
  verificationProbeKey,
  verificationProgressProbes,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationProbeProgress } from "./verification-probe-progress";

export function VerificationCombinationProgress(props: {
  result: VerificationRecord;
  active: boolean;
}): ReactElement {
  const result = props.result;
  const probes = verificationProgressProbes(result, props.active);
  const completed = rows(result.probes);
  const current = probes.find((probe) => probe.status === "running");
  const total = number(result.planned_probes) ?? number(result.total);
  const passed = completed.filter((probe) => probe.status === "passed").length;
  const failed = completed.filter(
    (probe) => probe.status === "failed" || probe.status === "error",
  ).length;
  const collected = completed.filter((probe) => probe.status === "observed").length;
  const groups = [
    { label: "能力检测", probes: probes.filter((probe) => probe.category !== "fingerprint") },
    { label: "分词指纹", probes: probes.filter((probe) => probe.category === "fingerprint") },
  ];
  return (
    <article className="min-w-0 space-y-4">
      <header className="space-y-2 border-b pb-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="font-medium wrap-anywhere">{text(result.account_name) || "账号"}</h3>
            <p className="text-muted-foreground mt-1 text-xs wrap-anywhere">
              ID {text(result.account_id)} · {text(result.claimed_model) || "模型"}
            </p>
          </div>
          <Badge variant="outline">{props.active ? "检测中" : "已结束"}</Badge>
        </div>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums text-muted-foreground">
          <span>
            已执行 {completed.length}/{total ?? "—"} 项
          </span>
          <span>通过 {passed}</span>
          <span className={failed > 0 ? "text-destructive" : undefined}>异常 {failed}</span>
          {collected > 0 ? <span>已采集 {collected}</span> : null}
        </div>
        {current ? (
          <p className="text-primary text-xs wrap-anywhere">当前检测：{text(current.label)}</p>
        ) : null}
        {text(result.error) ? (
          <p className="text-destructive text-sm wrap-anywhere">{text(result.error)}</p>
        ) : null}
        {rows(result.evidence)
          .filter((evidence) => evidence.id === "source")
          .map((evidence) => (
            <details key="source" className="text-xs text-muted-foreground">
              <summary className="cursor-pointer">请求来源</summary>
              <p className="mt-2 wrap-anywhere">{text(evidence.detail)}</p>
            </details>
          ))}
      </header>
      {groups
        .filter((group) => group.probes.length > 0)
        .map((group) => (
          <section key={group.label} aria-label={group.label} className="min-w-0 space-y-2">
            <div className="flex items-center justify-between gap-2 px-1">
              <h4 className="text-sm font-medium">{group.label}</h4>
              <span className="text-xs text-muted-foreground">{group.probes.length} 项</span>
            </div>
            <div className="min-w-0 divide-y overflow-hidden rounded-md border">
              {group.probes.map((probe) => (
                <VerificationProbeProgress
                  key={verificationProbeKey(probe)}
                  probe={probe}
                  timeout={number(result.timeout_seconds)}
                />
              ))}
            </div>
          </section>
        ))}
      {probes.length === 0 ? (
        <p className="text-muted-foreground py-4 text-sm">
          {props.active
            ? "等待单项检测进度，收到后会显示题目与回答。"
            : "此组合没有逐项记录，请查看最终报告。"}
        </p>
      ) : null}
    </article>
  );
}
