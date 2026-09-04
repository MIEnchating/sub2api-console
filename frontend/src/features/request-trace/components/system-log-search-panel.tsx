import { useState } from "react";
import { Clock3, FileSearch, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useQuery } from "@tanstack/react-query";

import { api, type SystemLogSearchQuery, type UsageRecord } from "@/api";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePagination } from "@/components/data-table/pagination";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { QueryErrorToast } from "@/components/query-error-toast";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

function emptySearch(): SystemLogSearchQuery {
  return {
    requestId: "",
    page: 1,
    pageSize: 20,
  };
}

function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === "") return "";
  return String(value);
}

function messageValues(message: string): { summary: string; values: Record<string, string> } {
  const values: Record<string, string> = {};
  const expression = /(?:^|\s)([a-zA-Z][\w.-]*)=([^\s]+)/g;
  let firstIndex = -1;
  for (const match of message.matchAll(expression)) {
    if (firstIndex < 0) firstIndex = match.index ?? -1;
    values[match[1]] = match[2];
  }
  return {
    summary: (firstIndex >= 0 ? message.slice(0, firstIndex) : message).trim(),
    values,
  };
}

function payloadValue(record: UsageRecord, keys: string[]): string {
  const extra =
    typeof record.payload.extra === "object" && record.payload.extra !== null
      ? (record.payload.extra as Record<string, unknown>)
      : {};
  for (const key of keys) {
    const direct = displayValue(record.payload[key]);
    if (direct) return direct;
    const nested = displayValue(extra[key]);
    if (nested) return nested;
  }
  return "";
}

function firstValue(values: Record<string, string>, keys: string[]): string {
  for (const key of keys) {
    if (values[key]) return values[key];
  }
  return "";
}

function readableTitle(summary: string): string {
  const normalized = summary.toLowerCase();
  if (normalized === "http request completed") return "HTTP 请求完成";
  if (normalized === "http request started") return "HTTP 请求开始";
  if (normalized === "http request failed") return "HTTP 请求失败";
  if (normalized.startsWith("account test error:")) return "账号测试失败";
  return summary || "系统日志";
}

function readableDuration(value: string): string {
  const milliseconds = Number(value);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return value || "未记录";
  if (milliseconds < 1000) return `${milliseconds} ms`;
  const seconds = (milliseconds / 1000).toFixed(3).replace(/\.?0+$/, "");
  return `${seconds} 秒 (${milliseconds} ms)`;
}

export type ReadableSystemLog = {
  title: string;
  status: string;
  duration: string;
  requestId: string;
  clientRequestId: string;
  accountId: string;
  accountName: string;
  account: string;
  apiKeyId: string;
  platform: string;
  model: string;
  method: string;
  path: string;
  ip: string;
  protocol: string;
  host: string;
};

type SystemLogSubmission = {
  query: SystemLogSearchQuery;
  execution: number;
};

export function nextSystemLogSubmission(
  current: SystemLogSubmission | null,
  query: SystemLogSearchQuery,
): SystemLogSubmission {
  return {
    query,
    execution: (current?.execution ?? 0) + 1,
  };
}

export function readableSystemLog(record: UsageRecord): ReadableSystemLog {
  const message = messageValues(record.summary ?? "");
  const fromMessage = (keys: string[]) => firstValue(message.values, keys);
  const fromPayload = (keys: string[]) => payloadValue(record, keys);
  const accountId =
    displayValue(record.account_id) ||
    fromPayload(["account_id"]) ||
    fromMessage(["acc", "account_id"]);
  const accountName = displayValue(record.account_name);
  return {
    title: readableTitle(message.summary),
    status: fromPayload(["status", "status_code"]) || fromMessage(["status", "status_code"]),
    duration:
      displayValue(record.duration_ms) ||
      fromPayload(["latency_ms", "duration_ms"]) ||
      fromMessage(["latency_ms", "duration_ms"]),
    requestId:
      displayValue(record.request_id) || fromPayload(["request_id"]) || fromMessage(["req"]),
    clientRequestId:
      fromPayload(["client_request_id"]) || fromMessage(["client_req", "client_request_id"]),
    accountId,
    accountName,
    account: readableAccount(accountName, accountId),
    apiKeyId: fromPayload(["api_key_id"]) || fromMessage(["api_key_id", "key"]),
    platform: fromPayload(["platform"]) || fromMessage(["platform"]),
    model: fromPayload(["model"]) || fromMessage(["model"]),
    method: fromPayload(["method"]) || fromMessage(["method"]),
    path: fromPayload(["path"]) || fromMessage(["path"]),
    ip: fromPayload(["ip", "client_ip"]) || fromMessage(["ip", "client_ip"]),
    protocol: fromPayload(["proto", "protocol"]) || fromMessage(["proto", "protocol"]),
    host: fromPayload(["host"]),
  };
}

function readableAccount(name: string, id: string): string {
  if (name && id) return `${name}（ID：${id}）`;
  if (name) return name;
  if (id) return `未知账号（ID：${id}）`;
  return "未记录";
}

function statusVariant(record: UsageRecord, status: string): "danger" | "warning" | "success" {
  const statusCode = Number(status);
  if (record.is_error || statusCode >= 500) return "danger";
  if (statusCode >= 400) return "warning";
  return "success";
}

function Field(props: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex min-w-0 items-center gap-3 text-xs font-medium">
      <span className="text-muted-foreground shrink-0 whitespace-nowrap">{props.label}</span>
      <span className="min-w-0 flex-1">{props.children}</span>
    </label>
  );
}

function DetailItem(props: { label: string; value: string; wide?: boolean; mono?: boolean }) {
  return (
    <div className={props.wide ? "min-w-0 sm:col-span-2" : "min-w-0"}>
      <dt className="text-muted-foreground text-xs">{props.label}</dt>
      <dd
        className={`mt-1 min-w-0 break-all text-sm ${props.mono ? "font-mono text-xs" : "font-medium"}`}
      >
        {props.value || "未记录"}
      </dd>
    </div>
  );
}

function SystemLogResult(props: { record: UsageRecord }) {
  const log = readableSystemLog(props.record);
  let statusLabel = "已完成";
  if (log.status) statusLabel = `HTTP ${log.status}`;
  else if (props.record.is_error) statusLabel = "失败";
  return (
    <article className="grid min-w-0 gap-4 px-4 py-4 sm:px-5">
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <strong className="text-sm">{log.title}</strong>
            <StatusBadge label={statusLabel} variant={statusVariant(props.record, log.status)} />
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
        {log.host ? <span className="text-muted-foreground text-xs">Host：{log.host}</span> : null}
      </div>

      <dl className="grid min-w-0 grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2 lg:grid-cols-4">
        <DetailItem label="请求 ID" value={log.requestId} wide mono />
        <DetailItem label="客户端请求 ID" value={log.clientRequestId} wide mono />
        <DetailItem label="账号" value={log.account} />
        <DetailItem label="KEY ID" value={log.apiKeyId} />
        <DetailItem label="平台" value={log.platform} />
        <DetailItem label="模型" value={log.model} />
        <DetailItem label="请求方法" value={log.method} />
        <DetailItem label="请求路径" value={log.path} mono />
        <DetailItem label="客户端 IP" value={log.ip} />
        <DetailItem label="HTTP 协议" value={log.protocol} />
      </dl>

      {props.record.summary ? (
        <div className="bg-muted/60 min-w-0 rounded-md px-3 py-2.5">
          <div className="text-muted-foreground mb-1 text-xs">原始日志</div>
          <code className="block min-w-0 break-words text-xs leading-5 whitespace-pre-wrap">
            {props.record.summary}
          </code>
        </div>
      ) : null}
    </article>
  );
}

export function SystemLogSearchPanel() {
  const [form, setForm] = useState<SystemLogSearchQuery>(emptySearch);
  const [submitted, setSubmitted] = useState<SystemLogSubmission | null>(null);
  const logs = useQuery({
    queryKey: ["sub2api-system-logs", submitted?.query, submitted?.execution],
    queryFn: () => api.systemLogs(submitted!.query),
    enabled: submitted !== null,
    retry: false,
  });

  function update<Key extends keyof SystemLogSearchQuery>(
    key: Key,
    value: SystemLogSearchQuery[Key],
  ) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const query = {
      ...form,
      requestId: form.requestId.trim(),
      page: 1,
    };
    setSubmitted((current) => nextSystemLogSubmission(current, query));
  }

  function reset() {
    setForm(emptySearch());
    setSubmitted(null);
  }

  function changePage(page: number) {
    setSubmitted((current) =>
      current ? { ...current, query: { ...current.query, page } } : current,
    );
  }

  function changePageSize(pageSize: number) {
    setForm((current) => ({ ...current, pageSize }));
    setSubmitted((current) =>
      current ? { ...current, query: { ...current.query, page: 1, pageSize } } : current,
    );
  }

  const totalPages = logs.data ? Math.max(1, Math.ceil(logs.data.total / logs.data.page_size)) : 1;
  const logPage = logs.data;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <TableFilterToolbar aria-label="系统日志查询">
        <form className="contents" onSubmit={submit}>
          <div className="min-w-0 basis-72 flex-1">
            <Field label="request_id">
              <Input
                value={form.requestId}
                onChange={(event) => update("requestId", event.target.value)}
                autoComplete="off"
                placeholder="输入完整 request_id"
              />
            </Field>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button type="submit" disabled={logs.isFetching}>
              {logs.isFetching ? <RefreshCw className="animate-spin" /> : <Search />}
              {logs.isFetching ? "查询中" : "查询"}
            </Button>
            <Button type="button" variant="outline" onClick={reset} disabled={logs.isFetching}>
              <RotateCcw />
              重置
            </Button>
          </div>
        </form>
      </TableFilterToolbar>

      {logs.isError ? (
        <QueryErrorToast error={logs.error} fallback="Sub2API 系统日志查询失败" />
      ) : null}
      {submitted && (
        <DataTablePanel className="mt-3 flex-1 sm:mt-4">
          {logs.isLoading && (
            <div className="text-muted-foreground flex min-h-28 items-center justify-center gap-2 text-sm">
              <RefreshCw className="size-4 animate-spin" />
              正在读取 Sub2API 系统日志
            </div>
          )}
          {!logs.isLoading && logPage && logPage.items.length > 0 && (
            <>
              <div className="min-h-0 flex-1 divide-y overflow-y-auto">
                {logPage.items.map((record) => (
                  <SystemLogResult
                    key={`${record.id}:${record.request_id}:${record.observed_at ?? ""}`}
                    record={record}
                  />
                ))}
              </div>
              <DataTablePagination
                currentPage={logPage.page}
                totalPages={totalPages}
                totalItems={logPage.total}
                pageSize={logPage.page_size}
                onPageChange={changePage}
                onPageSizeChange={changePageSize}
              />
            </>
          )}
          {!logs.isLoading && !logPage?.items.length && (
            <div className="text-muted-foreground flex min-h-28 items-center justify-center text-sm">
              没有匹配的系统日志
            </div>
          )}
        </DataTablePanel>
      )}
      {!submitted && (
        <div
          data-slot="request-trace-placeholder"
          className="text-muted-foreground flex min-h-0 flex-1 flex-col items-center justify-center gap-3"
        >
          <FileSearch className="size-8" aria-hidden="true" />
          <p className="text-sm">输入 request_id 开始查询</p>
        </div>
      )}
    </div>
  );
}
