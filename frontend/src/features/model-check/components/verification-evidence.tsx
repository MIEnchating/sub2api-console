import { useState, type ReactElement } from "react";
import { CircleHelp } from "lucide-react";
import { VerificationProbeNavigation } from "./verification-probe-navigation";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { verificationIdentityExplanation } from "../constants";
import {
  verificationNumber as number,
  verificationRecord as record,
  verificationRows as rows,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationFingerprintDetails } from "./verification-fingerprint-details";
import { VerificationProbeDetail } from "./verification-probe-detail";
import {
  VerificationCapabilityStatus,
  VerificationIdentityStatus,
  VerificationIdentitySummary,
} from "./verification-status";

export function VerificationEvidence(props: { result: VerificationRecord }): ReactElement {
  const [view, setView] = useState("probes");
  const [selected, setSelected] = useState(0);
  const probes = rows(props.result.probes);
  const current = probes[selected] ?? probes[0];
  const failedRequests = probes.filter((probe) => probe.status === "error");
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      <div className="shrink-0 space-y-3">
        <p className="text-base font-semibold wrap-anywhere">
          {text(props.result.account_name)}
          {props.result.account_name ? " · " : ""}
          {text(props.result.claimed_model)}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <VerificationCapabilityStatus status={text(record(props.result.capability).status)} />
          <VerificationIdentityStatus result={props.result} />
          <span className="text-muted-foreground text-xs tabular-nums">
            已执行 {probes.length}/{number(props.result.total) ?? "—"} 项 · 能力通过{" "}
            {number(props.result.passed) ?? "—"} 项
          </span>
        </div>
        {failedRequests.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="destructive">请求失败 {failedRequests.length} 项</Badge>
            <Button
              variant="ghost"
              onClick={() => {
                setSelected(probes.findIndex((probe) => probe.status === "error"));
                setView("probes");
              }}
            >
              查看失败项
            </Button>
          </div>
        ) : null}
        <VerificationIdentitySummary result={props.result} />
        <div className="flex gap-1 border-b pb-2" role="group" aria-label="证据视图">
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
      </div>
      {view === "probes" ? (
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-md border md:flex-row">
          <VerificationProbeNavigation probes={probes} selected={selected} onSelect={setSelected} />
          <div
            className="min-h-0 min-w-0 flex-1 overflow-auto"
            role="region"
            aria-label="检测项内容"
            tabIndex={0}
            key={selected}
          >
            {current ? (
              <VerificationProbeDetail probe={current} />
            ) : (
              <p className="p-4 text-sm text-muted-foreground">
                任务未返回题目与回答，请查看来源与指纹。
              </p>
            )}
          </div>
        </div>
      ) : (
        <section aria-label="来源与对照证据" className="min-h-0 flex-1 space-y-3 overflow-auto">
          <div className="bg-muted/30 rounded-md border p-3 text-sm">
            <p className="flex items-center gap-2 font-medium">
              <CircleHelp className="size-4 shrink-0" aria-hidden="true" />
              检测范围与结论
            </p>
            <p className="text-muted-foreground mt-2">{verificationIdentityExplanation}</p>
          </div>
          <VerificationFingerprintDetails result={props.result} />
          <div className="grid gap-3 md:grid-cols-2">
            {rows(props.result.evidence)
              .filter((item) => !(item.id === "reference" && item.status === "unknown"))
              .map((item, index) => (
                <article key={text(item.id) || index} className="rounded-md border p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <h3 className="text-sm font-medium">{text(item.label)}</h3>
                    {item.status === "warning" ? <Badge variant="warning">待核对</Badge> : null}
                  </div>
                  <p className="text-muted-foreground mt-2 text-sm leading-relaxed wrap-anywhere">
                    {text(item.detail)}
                  </p>
                </article>
              ))}
          </div>
        </section>
      )}
    </div>
  );
}
