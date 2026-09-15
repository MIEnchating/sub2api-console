import { useDeferredValue, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";

import {
  api,
  type UnifiedLogEntry,
  type UnifiedLogEventLevel,
  type UnifiedLogKind,
  type UnifiedLogState,
} from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { LogDetailsDialog } from "./log-details-dialog";
import { logKinds, LogsFilterToolbar } from "./logs-filter-toolbar";
import { LogsTable } from "./logs-table";

export function LogsCenterPage() {
  const searchParams = useSearch({ strict: false }) as { kind?: unknown };
  const navigate = useNavigate();
  const kind =
    typeof searchParams.kind === "string" && logKinds.includes(searchParams.kind as UnifiedLogKind)
      ? (searchParams.kind as UnifiedLogKind)
      : "all";
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
        action={
          <PageActions>
            <RefreshButton
              pending={logs.isFetching}
              ariaLabel="刷新日志"
              onClick={() => void logs.refetch()}
            />
          </PageActions>
        }
      />
      <div className="flex h-full min-h-0 flex-col gap-3">
        <LogsFilterToolbar
          search={search}
          kind={kind}
          state={state}
          eventLevel={eventLevel}
          eventGroup={eventGroup}
          groups={groups.data ?? []}
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
        />
        <DataTablePanel
          id="logs-results-panel"
          role="tabpanel"
          aria-labelledby={`logs-kind-tab-${kind}`}
          className="flex-1"
          data-testid="logs-table-shell"
        >
          <LogsTable
            items={logs.data?.items ?? []}
            kind={kind}
            loading={logs.isLoading}
            unavailable={!logs.data && logs.isError}
            refreshing={logs.isFetching}
            filtered={Boolean(
              search || state !== "all" || eventLevel !== "all" || eventGroup !== "all",
            )}
            onRetry={() => void logs.refetch()}
            onSelect={setSelected}
          />
          {(logs.data?.total ?? 0) > 0 && (
            <div className="shrink-0" data-testid="logs-pagination-region">
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
            </div>
          )}
        </DataTablePanel>
      </div>
      <LogDetailsDialog entry={selected} onClose={() => setSelected(null)} />
    </PageLayout>
  );
}
