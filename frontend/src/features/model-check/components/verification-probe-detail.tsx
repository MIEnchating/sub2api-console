import type { ReactElement } from "react";
import { JsonEditor } from "@/components/json-editor";
import { VerificationStateBadge } from "./verification-state";
import { verificationProbeLabels, verificationProtocolLabels } from "../constants";
import {
  verificationNumber as number,
  verificationRecord as record,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";

function OriginalText(props: { label: string; value: string; json: boolean }): ReactElement {
  return (
    <section className="min-w-0 space-y-2" aria-label={props.label}>
      <h4 className="text-muted-foreground text-xs font-medium">{props.label}</h4>
      {props.json ? (
        <JsonEditor aria-label={props.label} value={props.value} readOnly className="h-40" />
      ) : (
        <pre className="bg-muted/40 max-h-64 overflow-auto rounded-md border p-3 font-mono text-sm whitespace-pre-wrap wrap-anywhere">
          {props.value || "未记录"}
        </pre>
      )}
    </section>
  );
}

export function VerificationProbeDetail(props: { probe: VerificationRecord }): ReactElement {
  const probe = props.probe;
  const status = text(probe.status);
  const json = probe.id === "structured";
  return (
    <article className="min-w-0 space-y-4 p-3 sm:p-4" aria-label="检测项详情">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-medium wrap-anywhere">{text(probe.label)}</h3>
        <VerificationStateBadge
          status={status}
          label={verificationProbeLabels[status] ?? "待判定"}
        />
      </div>
      <p className="text-muted-foreground text-xs wrap-anywhere">
        第 {number(probe.round) ?? "—"} 轮 ·{" "}
        {verificationProtocolLabels[text(probe.protocol)] ?? "协议未记录"} · HTTP{" "}
        {number(probe.http_status) ?? "—"} · {number(probe.duration_ms) ?? "—"} ms
      </p>
      <section aria-label="检测题目" className="space-y-2">
        <h4 className="text-muted-foreground text-xs font-medium">检测题目</h4>
        <p className="max-h-48 overflow-auto text-sm whitespace-pre-wrap wrap-anywhere">
          {text(probe.prompt) || "题目未记录"}
        </p>
      </section>
      <div className="grid min-w-0 gap-4 lg:grid-cols-2">
        <OriginalText
          label={probe.category === "fingerprint" ? "采样目的" : "预期答案"}
          value={text(probe.expected)}
          json={json}
        />
        <OriginalText
          label="实际回答"
          value={text(probe.answer) || "未返回回答"}
          json={json && Boolean(text(probe.answer))}
        />
      </div>
      {text(probe.error) ? (
        <p className="text-destructive rounded-md border border-destructive/30 p-3 text-sm wrap-anywhere">
          {text(probe.error)}
        </p>
      ) : null}
      {probe.answer_truncated === true ? (
        <p className="text-muted-foreground text-xs">
          回答过长，仅展示截取内容；判定使用完整回答。
        </p>
      ) : null}
      <dl className="text-muted-foreground grid gap-2 border-t pt-3 text-xs">
        <div>
          <dt className="inline">返回型号：</dt>
          <dd className="inline wrap-anywhere">{text(probe.response_model) || "未返回"}</dd>
        </div>
        <div>
          <dt className="sr-only">Token 用量</dt>
          <dd>
            输入 {number(record(probe.usage).input_tokens) ?? "—"} / 输出{" "}
            {number(record(probe.usage).output_tokens) ?? "—"} Token
          </dd>
        </div>
        <div>
          <dt className="inline">请求 ID：</dt>
          <dd className="inline font-mono wrap-anywhere">{text(probe.request_id) || "未记录"}</dd>
        </div>
      </dl>
    </article>
  );
}
