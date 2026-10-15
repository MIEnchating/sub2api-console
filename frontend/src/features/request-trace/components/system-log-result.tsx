import { Clock3 } from "lucide-react";
import type { UsageRecord } from "@/api";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import { cn } from "@/lib/utils";
import { logLevelDisplay } from "../constants";
import { readableDuration, readableSystemLog, readableUsage } from "../lib/readable-log";
import { TraceAccountActions } from "./trace-account-actions";

function statusVariant(record: UsageRecord, status: string): StatusVariant {
  const code = Number(status);
  if (record.is_error || code >= 500) return "danger";
  if (code >= 400) return "warning";
  if (code >= 200 && code < 400) return "success";
  return "neutral";
}

function DetailItem(props: {
  label: string;
  value: string;
  wide?: boolean;
  mono?: boolean;
}): React.ReactElement {
  return (
    <div className={props.wide ? "min-w-0 sm:col-span-2" : "min-w-0"}>
      <dt className="text-muted-foreground text-xs">{props.label}</dt>
      <dd
        className={cn(
          "mt-1 min-w-0 break-all text-sm font-medium",
          props.mono && "font-mono text-xs font-normal",
        )}
      >
        {props.value || "未记录"}
      </dd>
    </div>
  );
}

export function SystemLogResult(props: { record: UsageRecord }): React.ReactElement {
  const log = readableSystemLog(props.record);
  const usage = readableUsage(props.record);
  const level = logLevelDisplay(log.level);
  let statusLabel = "";
  if (log.status) statusLabel = `HTTP ${log.status}`;
  else if (props.record.is_error) statusLabel = "失败";
  return (
    <article className="bg-card min-w-0 overflow-hidden rounded-lg border">
      <header className="bg-muted/30 flex min-w-0 flex-col gap-2 border-b px-3 py-3 sm:flex-row sm:items-start sm:justify-between sm:px-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="min-w-0 break-all text-sm font-semibold">{log.title}</h2>
            <StatusBadge label={level.label} variant={level.variant} />
            {statusLabel ? (
              <StatusBadge label={statusLabel} variant={statusVariant(props.record, log.status)} />
            ) : null}
            {log.duration ? (
              <StatusBadge label={`耗时 ${readableDuration(log.duration)}`} icon={Clock3} />
            ) : null}
          </div>
          <p className="text-muted-foreground mt-1 text-xs">
            {props.record.observed_at
              ? new Date(props.record.observed_at).toLocaleString("zh-CN", { hour12: false })
              : "时间未记录"}
          </p>
        </div>
        {log.host ? (
          <span className="text-muted-foreground min-w-0 break-all text-xs sm:max-w-[35%] sm:text-right">
            Host：{log.host}
          </span>
        ) : null}
      </header>

      <div className="grid min-w-0 gap-3 p-3 sm:p-4">
        <dl className="grid min-w-0 grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2 lg:grid-cols-4">
          <DetailItem label="请求 ID" value={log.requestId} wide mono />
          <DetailItem label="客户端请求 ID" value={log.clientRequestId} wide mono />
          <div className="min-w-0 sm:col-span-2 lg:col-span-1">
            <dt className="text-muted-foreground text-xs">账号</dt>
            <dd className="mt-1 min-w-0 break-all text-sm font-medium">{log.account}</dd>
          </div>
          <DetailItem label="KEY ID" value={log.apiKeyId} />
          <DetailItem label="平台" value={log.platform} />
          <DetailItem label="模型" value={log.model} />
        </dl>

        {log.method || log.path || log.ip || log.protocol ? (
          <dl className="grid min-w-0 grid-cols-2 gap-x-5 gap-y-3 border-t pt-3 lg:grid-cols-4">
            <DetailItem label="请求方法" value={log.method} />
            <DetailItem label="请求路径" value={log.path} mono />
            <DetailItem label="客户端 IP" value={log.ip} />
            <DetailItem label="HTTP 协议" value={log.protocol} />
          </dl>
        ) : null}

        {usage ? (
          <dl className="grid min-w-0 grid-cols-2 gap-x-5 gap-y-3 border-t pt-3 sm:grid-cols-4">
            <DetailItem label="输入 Token" value={usage.input} />
            <DetailItem label="输出 Token" value={usage.output} />
            <DetailItem label="缓存读取 Token" value={usage.cacheRead} />
            <DetailItem label="缓存写入 Token" value={usage.cacheWrite} />
          </dl>
        ) : null}

        {props.record.summary ? (
          <details className="bg-muted/40 min-w-0 rounded-md px-3 py-2.5">
            <summary className="text-muted-foreground focus-visible:ring-ring cursor-pointer rounded-sm text-xs outline-none focus-visible:ring-2">
              原始日志
            </summary>
            <code className="mt-2 block min-w-0 break-all text-xs leading-5 whitespace-pre-wrap">
              {props.record.summary}
            </code>
          </details>
        ) : null}
        {log.accountId ? (
          <div className="border-t pt-1">
            <TraceAccountActions key={log.accountId} accountId={log.accountId} />
          </div>
        ) : null}
      </div>
    </article>
  );
}
