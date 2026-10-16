import { useEffect, useState, type ReactElement } from "react";
import { VerificationProbeDetail } from "./verification-probe-detail";
import { VerificationStateBadge } from "./verification-state";
import { cn } from "@/lib/utils";
import { verificationProbeLabels, verificationProtocolLabels } from "../constants";
import {
  verificationNumber as number,
  verificationRecord as record,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";

function RequestElapsed(props: { startedAt: string; timeout: number | null }): ReactElement {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const started = Date.parse(props.startedAt);
  const seconds = Math.max(0, Math.floor((now - started) / 1000));
  return (
    <span className="text-muted-foreground text-xs tabular-nums">
      {Number.isFinite(started) ? `已等待 ${seconds} 秒` : "请求进行中"}
      {props.timeout !== null ? ` · 单次超时 ${props.timeout} 秒` : ""}
    </span>
  );
}

export function VerificationProbeProgress(props: {
  probe: VerificationRecord;
  timeout: number | null;
  compact?: boolean;
  showFullDetails?: boolean;
}): ReactElement {
  const [expanded, setExpanded] = useState(false);
  const probe = props.probe;
  const status = text(probe.status);
  const running = status === "running";
  const error = status === "error";
  const failed = error || status === "failed";
  const pending = status === "pending" || status === "skipped";
  const preview = text(probe.answer) || text(probe.error);
  return (
    <details
      onToggle={(event) => setExpanded(event.currentTarget.open)}
      className={cn("group min-w-0 px-3 py-2.5", running && "bg-primary/5")}
    >
      <summary className="cursor-pointer list-none rounded outline-none focus-visible:ring-2 focus-visible:ring-ring">
        <span className={cn("flex items-center gap-2 text-sm", !props.compact && "flex-wrap")}>
          <span
            title={text(probe.label)}
            className={cn(
              "min-w-0 font-medium",
              props.compact ? "flex-1 truncate" : "wrap-anywhere",
            )}
          >
            {text(probe.label) || "检测项"}
          </span>
          {!props.compact || (number(probe.round) ?? 1) > 1 ? (
            <span className="text-muted-foreground shrink-0 text-xs">
              第 {number(probe.round) ?? 1} 轮
            </span>
          ) : null}
          <VerificationStateBadge
            status={status}
            label={verificationProbeLabels[status] ?? "待判定"}
          />
          <span className="ml-auto shrink-0 text-xs text-muted-foreground group-open:hidden">
            展开
          </span>
          <span className="ml-auto hidden shrink-0 text-xs text-muted-foreground group-open:inline">
            收起
          </span>
        </span>
        {running ? (
          <RequestElapsed startedAt={text(probe.started_at)} timeout={props.timeout} />
        ) : null}
        {preview && (!props.compact || error) ? (
          <span
            className={
              failed
                ? "text-destructive mt-1 line-clamp-2 whitespace-pre-wrap wrap-anywhere text-xs group-open:line-clamp-none"
                : "text-muted-foreground mt-1 line-clamp-2 whitespace-pre-wrap wrap-anywhere text-xs group-open:line-clamp-none"
            }
          >
            {preview.length > 240 ? `${preview.slice(0, 240)}…` : preview}
          </span>
        ) : null}
      </summary>
      {props.showFullDetails ? (
        expanded && <VerificationProbeDetail probe={probe} />
      ) : (
        <div className="mt-3 space-y-2 border-t pt-3 text-sm">
          {!pending && !running ? (
            <p className="text-muted-foreground text-xs tabular-nums wrap-anywhere">
              {verificationProtocolLabels[text(probe.protocol)] ?? "协议未记录"} · HTTP{" "}
              {number(probe.http_status) ?? "—"} · {number(probe.duration_ms) ?? "—"} ms
            </p>
          ) : null}
          {props.compact ? <p className="font-medium wrap-anywhere">{text(probe.label)}</p> : null}
          <p className="text-muted-foreground">题目</p>
          <p className="max-h-48 overflow-auto whitespace-pre-wrap wrap-anywhere">
            {text(probe.prompt) || "题目未记录"}
          </p>
          <p className="wrap-anywhere">预期：{text(probe.expected) || "未记录"}</p>
          {props.compact && preview && !error ? (
            <p className="whitespace-pre-wrap wrap-anywhere">{preview}</p>
          ) : null}
          {preview.length > 240 && (!props.compact || failed) ? (
            <pre className="bg-muted max-h-64 overflow-auto rounded p-2 text-xs whitespace-pre-wrap wrap-anywhere">
              {preview}
            </pre>
          ) : null}
          {probe.answer_truncated === true ? (
            <p className="text-muted-foreground text-xs">
              回答过长，仅展示截取内容；判定使用完整回答。
            </p>
          ) : null}
          {running ? (
            <p className="text-muted-foreground">正在等待上游完整回复，收到后展示原文与判定。</p>
          ) : null}
          {!pending ? (
            <p className="text-muted-foreground text-xs wrap-anywhere">
              请求 ID：{text(probe.request_id) || "未记录"}
            </p>
          ) : null}
          {!pending && !running ? (
            <p className="text-muted-foreground text-xs wrap-anywhere">
              返回型号：{text(probe.response_model) || "未返回"} · 请求次数{" "}
              {number(probe.attempts) ?? "—"} · 输入{" "}
              {number(record(probe.usage).input_tokens) ?? "—"} / 输出{" "}
              {number(record(probe.usage).output_tokens) ?? "—"} Token
            </p>
          ) : null}
        </div>
      )}
    </details>
  );
}
