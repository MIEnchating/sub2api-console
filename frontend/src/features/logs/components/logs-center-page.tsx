import { useDeferredValue, useEffect, useState } from "react";
import { Eye, RefreshCw, Search } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";

import {
  api,
  type GroupStatus,
  type UnifiedLogEntry,
  type UnifiedLogEventLevel,
  type UnifiedLogKind,
  type UnifiedLogState,
} from "@/api";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  formatLogDate,
  logDetailRows,
  logEventLevel,
  logEventLevelLabel,
  logKindLabel,
  logSourceLabel,
  logStateLabel,
  logStatusLabel,
  logStatusVariant,
  logTitleLabel,
  relatedChanges,
  relatedEvents,
  type LogChangeRow,
} from "../lib/log-display";

const kinds: UnifiedLogKind[] = ["all", "task", "event", "change"];
const states: UnifiedLogState[] = ["all", "active", "failed", "warning", "succeeded"];
const eventLevels: UnifiedLogEventLevel[] = ["all", "info", "warning", "error"];

function normalizedKind(value: unknown): UnifiedLogKind {
  return typeof value === "string" && kinds.includes(value as UnifiedLogKind)
    ? (value as UnifiedLogKind)
    : "all";
}

export function LogKindFilter(props: {
  value: UnifiedLogKind;
  onChange: (value: UnifiedLogKind) => void;
}) {
  return (
    <Select value={props.value} onValueChange={(value) => value && props.onChange(value)}>
      <SelectTrigger className="w-36" aria-label="日志类型">
        <SelectValue>{logKindLabel(props.value)}</SelectValue>
      </SelectTrigger>
      <SelectContent align="start">
        {kinds.map((option) => (
          <SelectItem key={option} value={option}>
            {logKindLabel(option)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function LogStateFilter(props: {
  value: UnifiedLogState;
  onChange: (value: UnifiedLogState) => void;
}) {
  return (
    <Select value={props.value} onValueChange={(value) => value && props.onChange(value)}>
      <SelectTrigger className="w-32" aria-label="执行结果">
        <SelectValue>{logStateLabel(props.value)}</SelectValue>
      </SelectTrigger>
      <SelectContent align="start">
        {states.map((option) => (
          <SelectItem key={option} value={option}>
            {logStateLabel(option)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function EventLevelFilter(props: {
  value: UnifiedLogEventLevel;
  onChange: (value: UnifiedLogEventLevel) => void;
}) {
  return (
    <Select value={props.value} onValueChange={(value) => value && props.onChange(value)}>
      <SelectTrigger className="w-32" aria-label="事件级别">
        <SelectValue>{logEventLevelLabel(props.value)}</SelectValue>
      </SelectTrigger>
      <SelectContent align="start">
        {eventLevels.map((option) => (
          <SelectItem key={option} value={option}>
            {logEventLevelLabel(option)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function EventGroupFilter(props: {
  value: string;
  groups: GroupStatus[];
  onChange: (value: string) => void;
}) {
  return (
    <Select value={props.value} onValueChange={(value) => value && props.onChange(value)}>
      <SelectTrigger className="w-40" aria-label="事件分组">
        <SelectValue>{props.value === "all" ? "全部分组" : props.value}</SelectValue>
      </SelectTrigger>
      <SelectContent align="start">
        <SelectItem value="all">全部分组</SelectItem>
        {props.groups.map((group) => (
          <SelectItem key={group.id ?? group.name} value={group.name}>
            {group.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function LogChangesTable(props: { changes: LogChangeRow[] }) {
  return (
    <Table
      containerClassName="max-h-[28rem] overflow-auto rounded border"
      className="min-w-[72rem]"
    >
      <TableHeader>
        <TableRow>
          <TableHead className="w-36">时间</TableHead>
          <TableHead className="w-44">渠道</TableHead>
          <TableHead className="w-32">分组</TableHead>
          <TableHead className="w-40">操作</TableHead>
          <TableHead>变更</TableHead>
          <TableHead className="w-24">结果</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.changes.map((change) => (
          <TableRow key={change.id}>
            <TableCell className="text-xs">{formatLogDate(change.occurredAt)}</TableCell>
            <TableCell>
              <div className="grid min-w-0 gap-0.5">
                <span className="truncate font-medium">{change.object}</span>
                {change.objectId && (
                  <span className="text-muted-foreground text-xs">#{change.objectId}</span>
                )}
              </div>
            </TableCell>
            <TableCell className="text-muted-foreground text-xs">
              {change.groups.length ? change.groups.join("、") : "未记录"}
            </TableCell>
            <TableCell>
              <Badge variant="outline">{change.operation}</Badge>
            </TableCell>
            <TableCell overflowTooltip={false}>
              <Tooltip>
                <TooltipTrigger render={<p className="whitespace-nowrap text-xs leading-5" />}>
                  {change.change}
                </TooltipTrigger>
                <TooltipContent className="max-w-sm">{change.change}</TooltipContent>
              </Tooltip>
            </TableCell>
            <TableCell overflowTooltip={false}>
              <StatusBadge label={change.result} variant={logStatusVariant(change.status)} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export function LogsFilterToolbar(props: {
  search: string;
  kind: UnifiedLogKind;
  state: UnifiedLogState;
  eventLevel: UnifiedLogEventLevel;
  eventGroup: string;
  groups: GroupStatus[];
  fetching: boolean;
  truncated: boolean;
  onSearchChange: (value: string) => void;
  onKindChange: (value: UnifiedLogKind) => void;
  onStateChange: (value: UnifiedLogState) => void;
  onEventLevelChange: (value: UnifiedLogEventLevel) => void;
  onEventGroupChange: (value: string) => void;
  onRefresh: () => void;
}) {
  return (
    <TableFilterToolbar data-testid="logs-filter-toolbar">
      <div className="relative w-full sm:w-56">
        <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
        <Input
          className="pl-8"
          value={props.search}
          onChange={(event) => props.onSearchChange(event.target.value)}
          placeholder="搜索任务、对象或原因"
          aria-label="搜索日志"
        />
      </div>
      <LogKindFilter value={props.kind} onChange={props.onKindChange} />
      {props.kind === "event" ? (
        <>
          <EventLevelFilter value={props.eventLevel} onChange={props.onEventLevelChange} />
          <EventGroupFilter
            value={props.eventGroup}
            groups={props.groups}
            onChange={props.onEventGroupChange}
          />
        </>
      ) : (
        <LogStateFilter value={props.state} onChange={props.onStateChange} />
      )}
      <div className="ml-auto flex shrink-0 items-center gap-2">
        {props.truncated && <Badge variant="outline">仅显示最近记录</Badge>}
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          aria-label="刷新日志"
          onClick={props.onRefresh}
          disabled={props.fetching}
        >
          <RefreshCw className={props.fetching ? "animate-spin" : undefined} />
        </Button>
      </div>
    </TableFilterToolbar>
  );
}

function LogDetailsDialog(props: { entry: UnifiedLogEntry | null; onClose: () => void }) {
  const detailRows = props.entry ? logDetailRows(props.entry.details) : [];
  const events = props.entry ? relatedEvents(props.entry) : [];
  const changes = props.entry ? relatedChanges(props.entry) : [];
  return (
    <Dialog open={props.entry !== null} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent className="max-h-[min(46rem,calc(100svh-2rem))] min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-[min(96vw,76rem)]">
        <DialogHeader>
          <DialogTitle>{props.entry ? logTitleLabel(props.entry.title) : "记录详情"}</DialogTitle>
          <DialogDescription>
            {props.entry
              ? `${logKindLabel(props.entry.kind)} · ${formatLogDate(props.entry.occurred_at)}`
              : "查看日志记录"}
          </DialogDescription>
        </DialogHeader>
        {props.entry && (
          <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
            <section className="border-b pb-4">
              <h3 className="text-muted-foreground text-xs font-medium">执行摘要</h3>
              <p className="mt-1 text-sm leading-6">{props.entry.summary}</p>
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                <StatusBadge
                  label={logStatusLabel(props.entry.status)}
                  variant={logStatusVariant(props.entry.status)}
                />
                <span className="text-muted-foreground">
                  来源：{logSourceLabel(props.entry.source)}
                </span>
                {props.entry.actor && (
                  <span className="text-muted-foreground">执行人：{props.entry.actor}</span>
                )}
                {props.entry.object_label && (
                  <span className="text-muted-foreground">对象：{props.entry.object_label}</span>
                )}
              </div>
            </section>
            {detailRows.length > 0 && (
              <section>
                <h3 className="text-muted-foreground mb-1 text-xs font-medium">执行信息</h3>
                <dl className="divide-border divide-y">
                  {detailRows.map((row) => (
                    <div
                      className="grid gap-1 py-2.5 sm:grid-cols-[8rem_minmax(0,1fr)]"
                      key={row.key}
                    >
                      <dt className="text-muted-foreground">{row.label}</dt>
                      <dd className="break-words font-medium">{row.value}</dd>
                    </div>
                  ))}
                </dl>
              </section>
            )}
            {events.length > 0 && (
              <section className="border-t pt-4">
                <h3 className="text-muted-foreground mb-1 text-xs font-medium">关联的事件日志</h3>
                <div className="divide-border divide-y">
                  {events.map((event) => (
                    <div className="grid gap-1 py-2.5" key={event.id}>
                      <div className="flex items-center justify-between gap-3">
                        <span className="font-medium">{logTitleLabel(event.title)}</span>
                        <span className="text-muted-foreground text-xs">
                          {formatLogDate(event.occurred_at)}
                        </span>
                      </div>
                      <p className="text-muted-foreground text-xs leading-5">{event.summary}</p>
                    </div>
                  ))}
                </div>
              </section>
            )}
            {changes.length > 0 && (
              <section className="border-t pt-4">
                <h3 className="text-muted-foreground mb-1 text-xs font-medium">关联的数据变更</h3>
                <LogChangesTable changes={changes} />
              </section>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

export function LogsCenterPage() {
  const searchParams = useSearch({ strict: false }) as { kind?: unknown };
  const navigate = useNavigate();
  const kind = normalizedKind(searchParams.kind);
  const [state, setState] = useState<UnifiedLogState>("all");
  const [eventLevel, setEventLevel] = useState<UnifiedLogEventLevel>("all");
  const [eventGroup, setEventGroup] = useState("all");
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selected, setSelected] = useState<UnifiedLogEntry | null>(null);
  const groups = useQuery({
    queryKey: ["groups"],
    queryFn: api.groups,
    enabled: kind === "event",
  });
  const selectedGroupID = groups.data?.find((group) => group.name === eventGroup)?.id ?? "";
  const logs = useQuery({
    queryKey: [
      "logs",
      kind,
      state,
      eventLevel,
      eventGroup,
      selectedGroupID,
      deferredSearch,
      page,
      pageSize,
    ],
    queryFn: () =>
      api.logs({
        kind,
        state,
        level: kind === "event" ? eventLevel : "all",
        group: kind === "event" && eventGroup !== "all" ? eventGroup : "",
        groupId: kind === "event" ? selectedGroupID : "",
        search: deferredSearch,
        page,
        pageSize,
      }),
    refetchInterval: 15_000,
    placeholderData: (previous) => previous,
  });
  const totalPages = Math.max(1, Math.ceil((logs.data?.total ?? 0) / pageSize));
  useEffect(() => setPage((current) => Math.min(current, totalPages)), [totalPages]);

  function selectKind(nextKind: UnifiedLogKind) {
    setPage(1);
    setState("all");
    setEventLevel("all");
    setEventGroup("all");
    void navigate({ to: "/logs", search: { kind: nextKind }, replace: true });
  }

  return (
    <PageLayout fixedContent>
      {logs.error && <QueryErrorToast error={logs.error} fallback="日志读取失败" />}
      <PageHeading
        eyebrow="OBSERVABILITY / LOGS"
        title="日志中心"
        description="统一查看任务、事件日志以及远程读取、写入和写后复核。"
      />
      <div className="flex h-full min-h-0 flex-col gap-3">
        <LogsFilterToolbar
          search={search}
          kind={kind}
          state={state}
          eventLevel={eventLevel}
          eventGroup={eventGroup}
          groups={groups.data ?? []}
          fetching={logs.isFetching}
          truncated={logs.data?.truncated === true}
          onSearchChange={(value) => {
            setSearch(value);
            setPage(1);
          }}
          onKindChange={selectKind}
          onStateChange={(value) => {
            setState(value);
            setPage(1);
          }}
          onEventLevelChange={(value) => {
            setEventLevel(value);
            setPage(1);
          }}
          onEventGroupChange={(value) => {
            setEventGroup(value);
            setPage(1);
          }}
          onRefresh={() => void logs.refetch()}
        />
        <Card className="min-h-0 flex-1 gap-0 py-0">
          <Table containerClassName="min-h-0 flex-1 overflow-auto" className="min-w-[920px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-40">时间</TableHead>
                <TableHead className="w-28">类型</TableHead>
                <TableHead>记录</TableHead>
                <TableHead className="w-44">对象 / 执行人</TableHead>
                <TableHead className="w-24">{kind === "event" ? "级别" : "状态"}</TableHead>
                <TableHead className="w-16 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.isLoading &&
                Array.from({ length: 6 }, (_, index) => (
                  <TableRow key={index} aria-label="正在加载日志">
                    {Array.from({ length: 6 }, (_, column) => (
                      <TableCell key={column}>
                        <Skeleton className="h-4 w-4/5" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              {!logs.isLoading && !logs.error && !logs.data?.items.length && (
                <TableRow>
                  <TableCell
                    colSpan={6}
                    className="h-28 text-center text-muted-foreground"
                    overflowTooltip={false}
                  >
                    {search || state !== "all" || eventLevel !== "all" || eventGroup !== "all"
                      ? "没有匹配的记录"
                      : "暂无日志记录"}
                  </TableCell>
                </TableRow>
              )}
              {!logs.error &&
                logs.data?.items.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell className="text-xs">{formatLogDate(entry.occurred_at)}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{logKindLabel(entry.kind)}</Badge>
                    </TableCell>
                    <TableCell tooltipContent={`${logTitleLabel(entry.title)}：${entry.summary}`}>
                      <div className="grid min-w-0 gap-0.5">
                        <span className="truncate font-medium">{logTitleLabel(entry.title)}</span>
                        <span className="text-muted-foreground truncate text-xs">
                          {entry.summary}
                          {entry.related_count > 0 ? ` · 关联 ${entry.related_count} 条` : ""}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell
                      tooltipContent={[entry.object_label, entry.actor].filter(Boolean).join(" · ")}
                    >
                      <div className="grid min-w-0 gap-0.5">
                        <span className={entry.object_label ? "truncate" : "text-muted-foreground"}>
                          {entry.object_label ?? "未记录对象"}
                        </span>
                        <span className="text-muted-foreground truncate text-xs">
                          {entry.actor ? `执行人：${entry.actor}` : logSourceLabel(entry.source)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell overflowTooltip={false}>
                      <StatusBadge
                        label={
                          entry.kind === "event"
                            ? logEventLevelLabel(logEventLevel(entry.status))
                            : logStatusLabel(entry.status)
                        }
                        variant={logStatusVariant(entry.status)}
                      />
                    </TableCell>
                    <TableCell className="text-right" overflowTooltip={false}>
                      <TableActionButton label="查看日志详情" onClick={() => setSelected(entry)}>
                        <Eye />
                      </TableActionButton>
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
          {!logs.error && (logs.data?.total ?? 0) > 0 && (
            <DataTablePagination
              currentPage={page}
              totalPages={totalPages}
              totalItems={logs.data?.total ?? 0}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(value) => {
                setPageSize(value);
                setPage(1);
              }}
            />
          )}
        </Card>
      </div>
      <LogDetailsDialog entry={selected} onClose={() => setSelected(null)} />
    </PageLayout>
  );
}
