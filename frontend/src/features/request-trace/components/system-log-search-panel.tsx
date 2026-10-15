import { useState } from "react";
import { FileSearch } from "lucide-react";
import { useQuery } from "@tanstack/react-query";

import { api, type SystemLogSearchQuery } from "@/api";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { DataTablePagination } from "@/components/data-table/pagination";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { QueryErrorToast } from "@/components/query-error-toast";
import { ContentRetry } from "@/components/content-retry";
import type { SystemLogFilters } from "../lib/search-schema";
import { SystemLogFiltersForm } from "./system-log-filters";
import { SystemLogResult } from "./system-log-result";

export function SystemLogSearchPanel(): React.ReactElement {
  const [pageSize, setPageSize] = useState(20);
  const [submitted, setSubmitted] = useState<SystemLogSearchQuery | null>(null);
  const logs = useQuery({
    queryKey: ["sub2api-system-logs", submitted],
    queryFn: (context) => api.systemLogs(submitted!, context.signal),
    enabled: submitted !== null,
    retry: false,
  });

  function submit(filters: SystemLogFilters): void {
    if (logs.isFetching) return;
    const query = {
      ...filters,
      pageSize,
      page: 1,
    };
    if (
      submitted?.requestId === query.requestId &&
      submitted.timeRange === query.timeRange &&
      submitted.level === query.level &&
      submitted.page === query.page &&
      submitted.pageSize === query.pageSize
    ) {
      void logs.refetch();
    } else {
      setSubmitted(query);
    }
  }

  function reset(): void {
    setPageSize(20);
    setSubmitted(null);
  }

  function changePage(page: number): void {
    setSubmitted((current) => (current ? { ...current, page } : current));
  }

  function changePageSize(pageSize: number): void {
    setPageSize(pageSize);
    setSubmitted((current) => (current ? { ...current, page: 1, pageSize } : current));
  }

  const totalPages = logs.data ? Math.max(1, Math.ceil(logs.data.total / logs.data.page_size)) : 1;
  const logPage = logs.data;

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col">
      <SystemLogFiltersForm pending={logs.isFetching} onSearch={submit} onReset={reset} />

      {logs.isError ? (
        <QueryErrorToast error={logs.error} fallback="Sub2API 系统日志查询失败" />
      ) : null}
      <DataTablePanel className="mt-3 flex-1 sm:mt-4">
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          {logs.isLoading && (
            <PageLoadingSkeleton
              label="正在读取 Sub2API 系统日志"
              variant="list"
              fill
              framed={false}
            />
          )}
          {logs.isError && !logPage ? (
            <ContentRetry pending={logs.isFetching} onRetry={() => void logs.refetch()} />
          ) : null}
          {!logs.isLoading && logPage && (
            <>
              {logPage.items.length > 0 ? (
                <div
                  aria-label="日志查询结果"
                  className="bg-muted/20 min-h-0 min-w-0 flex-1 space-y-3 overflow-y-auto overscroll-contain p-3"
                >
                  {logPage.items.map((record) => (
                    <SystemLogResult
                      key={`${record.id}:${record.request_id}:${record.observed_at ?? ""}`}
                      record={record}
                    />
                  ))}
                </div>
              ) : (
                <div className="text-muted-foreground flex min-h-0 flex-1 items-center justify-center px-4 py-4 text-sm">
                  {logPage.total > 0 ? "当前页没有系统日志" : "没有匹配的系统日志"}
                </div>
              )}
            </>
          )}
          {!submitted && (
            <div
              data-slot="request-trace-placeholder"
              className="text-muted-foreground flex min-h-0 flex-1 flex-col items-center justify-center gap-3"
            >
              <FileSearch className="size-8" aria-hidden="true" />
              <p className="text-sm">设置筛选条件后点击查询</p>
              <p className="text-xs">可按时间和级别浏览，或输入请求 ID 精确定位</p>
            </div>
          )}
        </div>
        <DataTablePagination
          currentPage={logPage?.page ?? submitted?.page ?? 1}
          totalPages={totalPages}
          totalItems={logPage?.total ?? 0}
          pageSize={logPage?.page_size ?? pageSize}
          disabled={logs.isFetching}
          onPageChange={changePage}
          onPageSizeChange={changePageSize}
        />
      </DataTablePanel>
    </div>
  );
}
