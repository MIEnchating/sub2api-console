import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";

import { api, type UnifiedLogEntry } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { taskPollInterval } from "@/lib/task-state";
import {
  formatLogDate,
  formatLogDurationSeconds,
  logDetailLabel,
  logDetailRows,
  logEventLevel,
  logEventLevelLabel,
  logKindLabel,
  logSourceLabel,
  logStatusLabel,
  logStatusVariant,
  logTitleLabel,
  relatedChanges,
  relatedEvents,
  type LogChangeRow,
  type LogDetailRow,
} from "../lib/log-display";

type LogDialogWidth = "medium" | "wide";
type LogRecord = Record<string, unknown>;

const operationCollectionKeys = new Set([
  "active_operations",
  "completed_operations",
  "operations",
  "planned_operations",
]);
const recordMapKeys = new Set(["account_decisions", "account_targets"]);

function isLogRecord(value: unknown): value is LogRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isStructuredValue(value: unknown): boolean {
  return Array.isArray(value) || isLogRecord(value);
}

function hasStructuredContent(value: unknown): boolean {
  if (Array.isArray(value)) return value.length > 0;
  return isLogRecord(value) && Object.keys(value).length > 0;
}

function hasRecordCollection(value: unknown): boolean {
  if (Array.isArray(value)) return value.length > 0;
  if (!isLogRecord(value)) return false;
  return Object.values(value).some((item) => Array.isArray(item) && item.length > 0);
}

export function logDetailsDialogWidth(
  kind?: UnifiedLogEntry["kind"],
  details?: Record<string, unknown>,
): LogDialogWidth {
  if (kind === "change") return "wide";
  if (kind === "task") return "wide";
  return details && hasRecordCollection(details) ? "wide" : "medium";
}

function detailSectionLabel(kind: UnifiedLogEntry["kind"]): string {
  if (kind === "task") return "任务信息";
  if (kind === "event") return "事件信息";
  return "操作信息";
}

function summarySectionLabel(kind: UnifiedLogEntry["kind"]): string {
  if (kind === "task") return "任务摘要";
  if (kind === "event") return "事件摘要";
  return "操作摘要";
}

function detailRow(key: string, value: unknown): LogDetailRow {
  return logDetailRows({ [key]: value })[0] ?? { key, label: key, value: String(value) };
}

function scalarDetailRows(details: Record<string, unknown>): LogDetailRow[] {
  return Object.entries(details)
    .filter(([, value]) => !isStructuredValue(value))
    .flatMap(([key, value]) => logDetailRows({ [key]: value }));
}

function structuredDetailEntries(details: Record<string, unknown>): [string, unknown][] {
  const hasCompletedTimings =
    Array.isArray(details.operation_timings) && details.operation_timings.length > 0;
  return Object.entries(details).filter(([key, value]) => {
    if (["events", "changes", "runs"].includes(key) || !hasStructuredContent(value)) return false;
    return !hasCompletedTimings || !operationCollectionKeys.has(key);
  });
}

function recordMapRows(value: LogRecord): LogRecord[] {
  return Object.entries(value).flatMap(([key, item]) => {
    if (!isLogRecord(item)) return [];
    if (item.account_id !== undefined) return [item];
    return [{ ...item, account_id: key }];
  });
}

function structuredCollectionCount(key: string, value: unknown): number | null {
  if (Array.isArray(value) && value.some(isLogRecord)) return value.length;
  if (recordMapKeys.has(key) && isLogRecord(value)) return Object.keys(value).length;
  return null;
}

function recordTitle(record: LogRecord, index: number): string {
  for (const key of ["account_name", "object_name", "name", "host", "upstream_host"]) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  const identifier = record.account_id ?? record.object_id ?? record.id;
  return identifier === undefined ? `第 ${index + 1} 项` : `记录 ${String(identifier)}`;
}

function recordStatus(record: LogRecord): string | null {
  for (const key of ["status", "state", "result"]) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  return null;
}

function recordStatusVariant(status: string): StatusVariant {
  if (/失败|错误|异常/.test(status)) return "danger";
  if (/降级|跳过|警告|未知|未确认/.test(status)) return "warning";
  if (/成功|完成|一致|正常|通过|已同步/.test(status)) return "success";
  return logStatusVariant(status);
}

function visibleRecordRows(record: LogRecord): LogDetailRow[] {
  const hiddenKeys = new Set([
    "account_name",
    "object_name",
    "name",
    "host",
    "upstream_host",
    "status",
    "state",
    "result",
  ]);
  return Object.entries(record)
    .filter(([key, value]) => !hiddenKeys.has(key) && !isStructuredValue(value))
    .flatMap(([key, value]) => logDetailRows({ [key]: value }));
}

function LogResultItems(props: { values: unknown[] }): ReactElement {
  const records = props.values.filter(isLogRecord);
  const pagination = useClientPagination(records, 10);

  return (
    <div data-slot="log-result-items" className="overflow-hidden rounded-md border">
      <div className="divide-border divide-y">
        {pagination.visibleItems.map((record, index) => {
          const absoluteIndex = (pagination.currentPage - 1) * pagination.pageSize + index;
          const status = recordStatus(record);
          const rows = visibleRecordRows(record);
          const nested = Object.entries(record).filter(([, value]) => isStructuredValue(value));
          return (
            <article className="min-w-0 p-3" key={String(record.id ?? record.account_id ?? index)}>
              <div className="flex min-w-0 items-start justify-between gap-3">
                <strong className="min-w-0 break-words font-medium">
                  {recordTitle(record, absoluteIndex)}
                </strong>
                {status ? (
                  <StatusBadge
                    label={logStatusLabel(status)}
                    variant={recordStatusVariant(status)}
                  />
                ) : null}
              </div>
              {rows.length > 0 ? (
                <dl className="mt-2 grid gap-x-5 sm:grid-cols-2 lg:grid-cols-3">
                  {rows.map((row) => (
                    <div className="border-border min-w-0 border-t py-2" key={row.key}>
                      <dt className="text-muted-foreground text-xs">{row.label}</dt>
                      <dd className="mt-0.5 break-words text-xs leading-5 font-medium">
                        {row.value}
                      </dd>
                    </div>
                  ))}
                </dl>
              ) : null}
              {nested.map(([key, value]) => (
                <section className="border-border mt-2 border-t pt-2" key={key}>
                  <h5 className="text-muted-foreground text-xs font-medium">
                    {logDetailLabel(key)}
                  </h5>
                  <LogStructuredValue value={value} fieldKey={key} />
                </section>
              ))}
            </article>
          );
        })}
      </div>
      {records.length > pagination.pageSize ? (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={records.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 50, 100]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      ) : null}
    </div>
  );
}

function inspectionOperationLabel(value: unknown): string | null {
  if (typeof value === "string" && value.trim()) return logTitleLabel(value);
  if (!isLogRecord(value)) return null;
  const operation = value.operation;
  if (typeof operation === "string" && operation.trim()) return logTitleLabel(operation);
  const label = value.label;
  return typeof label === "string" && label.trim() ? label : null;
}

function LogOperationList(props: { values: unknown[] }): ReactElement {
  const labels = props.values
    .map(inspectionOperationLabel)
    .filter((label): label is string => label !== null);
  return (
    <p
      data-slot="log-operation-list"
      className="mt-1 break-words rounded-md border px-3 py-2 text-xs leading-5"
    >
      {labels.length > 0 ? labels.join("、") : "无"}
    </p>
  );
}

type InspectionOperationTiming = {
  operation: string;
  durationSeconds: unknown;
  startedAt: string | null;
};

function inspectionOperationTimings(values: unknown[]): InspectionOperationTiming[] {
  return values.flatMap((value) => {
    if (!isLogRecord(value) || typeof value.operation !== "string") return [];
    return [
      {
        operation: value.operation,
        durationSeconds: value.duration_seconds,
        startedAt: typeof value.started_at === "string" ? value.started_at : null,
      },
    ];
  });
}

function LogOperationTimings(props: { values: unknown[] }): ReactElement {
  const timings = inspectionOperationTimings(props.values);
  return (
    <div data-slot="log-operation-timings" className="mt-1 overflow-hidden rounded-md border">
      <table className="w-full table-fixed text-left text-xs">
        <thead className="bg-muted/40 text-muted-foreground">
          <tr>
            <th className="w-1/2 px-3 py-2 font-medium">巡检步骤</th>
            <th className="w-1/5 px-3 py-2 font-medium">耗时</th>
            <th className="w-[30%] px-3 py-2 font-medium">开始时间</th>
          </tr>
        </thead>
        <tbody className="divide-border divide-y">
          {timings.map((timing, index) => (
            <tr key={`${timing.operation}-${index}`}>
              <td className="break-words px-3 py-2 font-medium">
                {logTitleLabel(timing.operation)}
              </td>
              <td className="px-3 py-2 whitespace-nowrap">
                {formatLogDurationSeconds(timing.durationSeconds)}
              </td>
              <td className="px-3 py-2 whitespace-nowrap">
                {timing.startedAt ? formatLogDate(timing.startedAt) : "未记录"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function LogScalarList(props: { values: unknown[]; fieldKey?: string }): ReactElement {
  return (
    <ul className="divide-border mt-1 divide-y rounded-md border">
      {props.values.map((value, index) => (
        <li className="break-words px-3 py-2 text-xs leading-5" key={index}>
          {detailRow(props.fieldKey ?? "value", value).value}
        </li>
      ))}
    </ul>
  );
}

function LogStructuredSection(props: { fieldKey: string; value: unknown }): ReactElement {
  const count = structuredCollectionCount(props.fieldKey, props.value);
  if (count !== null && count > 10) {
    return (
      <details>
        <summary className="text-muted-foreground flex cursor-pointer items-center justify-between gap-3 border-y py-2 text-xs font-medium">
          <span>{logDetailLabel(props.fieldKey)}</span>
          <span className="shrink-0 tabular-nums">{count} 项</span>
        </summary>
        <LogStructuredValue value={props.value} fieldKey={props.fieldKey} />
      </details>
    );
  }
  return (
    <section>
      <h4 className="text-muted-foreground mb-1 text-xs font-medium">
        {logDetailLabel(props.fieldKey)}
      </h4>
      <LogStructuredValue value={props.value} fieldKey={props.fieldKey} />
    </section>
  );
}

export function LogStructuredValue(props: { value: unknown; fieldKey?: string }): ReactElement {
  if (Array.isArray(props.value)) {
    if (props.fieldKey === "operation_timings") {
      return <LogOperationTimings values={props.value} />;
    }
    if (props.fieldKey && operationCollectionKeys.has(props.fieldKey)) {
      return <LogOperationList values={props.value} />;
    }
    if (props.value.some(isLogRecord)) return <LogResultItems values={props.value} />;
    return <LogScalarList values={props.value} fieldKey={props.fieldKey} />;
  }
  if (!isLogRecord(props.value)) {
    return <p className="mt-1 break-words text-xs leading-5">{String(props.value)}</p>;
  }
  if (props.fieldKey && recordMapKeys.has(props.fieldKey)) {
    return <LogResultItems values={recordMapRows(props.value)} />;
  }

  const scalarRows = scalarDetailRows(props.value);
  const nestedEntries = structuredDetailEntries(props.value);
  return (
    <div className="mt-1 space-y-3">
      {scalarRows.length > 0 ? (
        <dl
          data-slot="log-result-summary"
          className="grid overflow-hidden rounded-md border sm:grid-cols-2 lg:grid-cols-3"
        >
          {scalarRows.map((row) => (
            <div className="border-border min-w-0 border-b px-3 py-2.5" key={row.key}>
              <dt className="text-muted-foreground text-xs">{row.label}</dt>
              <dd className="mt-0.5 break-words text-sm font-medium">{row.value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {nestedEntries.map(([key, value]) => (
        <LogStructuredSection fieldKey={key} value={value} key={key} />
      ))}
    </div>
  );
}

export function LogChangesList(props: { changes: LogChangeRow[] }): ReactElement {
  return (
    <div data-slot="log-change-list" className="divide-border divide-y rounded-md border">
      {props.changes.map((change) => (
        <article className="min-w-0 p-3" key={change.id}>
          <div className="flex min-w-0 items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="break-words font-medium">
                {change.object}
                {change.objectId ? (
                  <span className="text-muted-foreground ml-1 text-xs">#{change.objectId}</span>
                ) : null}
              </p>
              <div className="text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs">
                <span>{formatLogDate(change.occurredAt)}</span>
                <span>{change.operation}</span>
                <span>分组：{change.groups.length ? change.groups.join("、") : "未记录"}</span>
              </div>
            </div>
            <StatusBadge label={change.result} variant={logStatusVariant(change.status)} />
          </div>
          <p className="bg-muted/40 mt-3 break-words rounded-md px-3 py-2 text-xs leading-5">
            {change.change}
          </p>
        </article>
      ))}
    </div>
  );
}

export function LogDetailsContent(props: {
  entry: UnifiedLogEntry;
  details?: Record<string, unknown>;
  loading?: boolean;
  loadFailed?: boolean;
}): ReactElement {
  const details = props.details ?? props.entry.details;
  const duplicateKey = props.entry.kind === "task" ? "operation" : "event_type";
  const detailRows = scalarDetailRows(details).filter(
    (row) => props.entry.kind === "change" || row.key !== duplicateKey,
  );
  const structuredEntries = structuredDetailEntries(details);
  const events = relatedEvents(props.entry);
  const changes = relatedChanges(props.entry);
  const statusLabel =
    props.entry.kind === "event"
      ? logEventLevelLabel(logEventLevel(props.entry.status))
      : logStatusLabel(props.entry.status);
  const statusHeading = props.entry.kind === "event" ? "事件级别" : "执行结果";

  return (
    <DialogBody className="space-y-4" data-kind={props.entry.kind}>
      <section className="border-b pb-4" aria-labelledby="log-summary-heading">
        <h3 id="log-summary-heading" className="text-muted-foreground text-xs font-medium">
          {summarySectionLabel(props.entry.kind)}
        </h3>
        <p className="mt-1 break-words text-sm leading-6">{props.entry.summary}</p>
      </section>

      <section className="border-b pb-4" aria-labelledby="log-overview-heading">
        <h3 id="log-overview-heading" className="text-muted-foreground text-xs font-medium">
          {detailSectionLabel(props.entry.kind)}
        </h3>
        <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-4">
          <div className="min-w-0">
            <dt className="text-muted-foreground text-xs">{statusHeading}</dt>
            <dd className="mt-1">
              <StatusBadge label={statusLabel} variant={logStatusVariant(props.entry.status)} />
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-muted-foreground text-xs">记录时间</dt>
            <dd className="mt-1 break-words text-xs font-medium">
              {formatLogDate(props.entry.occurred_at)}
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-muted-foreground text-xs">数据来源</dt>
            <dd className="mt-1 break-words text-xs font-medium">
              {logSourceLabel(props.entry.source)}
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-muted-foreground text-xs">
              {props.entry.object_label ? "相关对象" : "执行人"}
            </dt>
            <dd className="mt-1 break-words text-xs font-medium">
              {props.entry.object_label ?? props.entry.actor ?? "系统自动执行"}
            </dd>
          </div>
        </dl>
      </section>

      {props.loading ? (
        <p className="text-muted-foreground text-xs" role="status">
          正在读取完整任务结果…
        </p>
      ) : null}
      {props.loadFailed ? (
        <p className="text-destructive text-xs" role="alert">
          完整任务结果读取失败，当前显示日志摘要。
        </p>
      ) : null}

      {detailRows.length > 0 ? (
        <section aria-labelledby="log-detail-heading">
          <h3 id="log-detail-heading" className="text-muted-foreground text-xs font-medium">
            详细信息
          </h3>
          <dl className="mt-1 grid gap-x-5 sm:grid-cols-2">
            {detailRows.map((row) => (
              <div className="border-border min-w-0 border-t py-2.5" key={row.key}>
                <dt className="text-muted-foreground text-xs">{row.label}</dt>
                <dd className="mt-1 break-words text-sm leading-5 font-medium">{row.value}</dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}

      {structuredEntries.map(([key, value]) => (
        <section className="border-t pt-4" key={key}>
          <h3 className="text-muted-foreground text-xs font-medium">{logDetailLabel(key)}</h3>
          <LogStructuredValue value={value} fieldKey={key} />
        </section>
      ))}

      {events.length > 0 ? (
        <section className="border-t pt-4" aria-labelledby="related-events-heading">
          <h3 id="related-events-heading" className="text-muted-foreground text-xs font-medium">
            关联事件
          </h3>
          <div className="divide-border mt-1 divide-y">
            {events.map((event) => (
              <article className="grid gap-1 py-2.5" key={event.id}>
                <div className="flex items-start justify-between gap-3">
                  <span className="break-words font-medium">{logTitleLabel(event.title)}</span>
                  <StatusBadge
                    label={logEventLevelLabel(logEventLevel(event.status))}
                    variant={logStatusVariant(event.status)}
                  />
                </div>
                <p className="text-muted-foreground break-words text-xs leading-5">
                  {event.summary}
                </p>
                <span className="text-muted-foreground text-xs">
                  {formatLogDate(event.occurred_at)}
                </span>
              </article>
            ))}
          </div>
        </section>
      ) : null}

      {changes.length > 0 ? (
        <section className="border-t pt-4" aria-labelledby="related-changes-heading">
          <h3
            id="related-changes-heading"
            className="text-muted-foreground mb-2 text-xs font-medium"
          >
            关联远程读写
          </h3>
          <LogChangesList changes={changes} />
        </section>
      ) : null}
    </DialogBody>
  );
}

export function LogDetailsDialog(props: {
  entry: UnifiedLogEntry | null;
  onClose: () => void;
}): ReactElement {
  const taskDetail = useQuery({
    queryKey: ["task", props.entry?.source_id],
    queryFn: () => api.task(props.entry!.source_id),
    enabled: props.entry?.source === "task",
    retry: false,
    refetchInterval: (query) =>
      props.entry?.source === "task" ? taskPollInterval(query, 1_000) : false,
  });
  const details = props.entry ? { ...props.entry.details } : {};
  if (props.entry?.source === "task" && taskDetail.data) details.result = taskDetail.data.result;

  return (
    <Dialog open={props.entry !== null} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent
        width={logDetailsDialogWidth(props.entry?.kind, details)}
        height="adaptive"
        className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>{props.entry ? logTitleLabel(props.entry.title) : "记录详情"}</DialogTitle>
          <DialogDescription>
            {props.entry
              ? `${logKindLabel(props.entry.kind)} · ${formatLogDate(props.entry.occurred_at)}`
              : "查看日志记录"}
          </DialogDescription>
        </DialogHeader>
        {props.entry ? (
          <LogDetailsContent
            entry={props.entry}
            details={details}
            loading={taskDetail.isLoading}
            loadFailed={taskDetail.isError}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
