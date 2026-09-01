import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import type { UnifiedLogPage } from "@/api";

vi.mock("@tanstack/react-router", async () => {
  const actual =
    await vi.importActual<typeof import("@tanstack/react-router")>("@tanstack/react-router");
  return {
    ...actual,
    useNavigate: () => () => Promise.resolve(),
    useSearch: () => ({}),
  };
});

import { LogsCenterPage } from "../components/logs-center-page";

const emptyLogs: UnifiedLogPage = {
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
  counts: {},
  truncated: false,
};

function renderLogsPage(logs: UnifiedLogPage = emptyLogs): string {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  });
  queryClient.setQueryData(["logs", "all", "all", "all", "all", "", "", 1, 20], logs);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <LogsCenterPage />
    </QueryClientProvider>,
  );
}

function openingTag(markup: string, attribute: string): string {
  const match = markup.match(new RegExp(`<[^>]+${attribute}[^>]*>`));
  expect(match).not.toBeNull();
  return match?.[0] ?? "";
}

describe("LogsCenterPage layout", () => {
  it("places the log filter toolbar above the table card", () => {
    const markup = renderLogsPage();
    const toolbarStart = markup.indexOf('data-slot="table-filter-toolbar"');
    const cardStart = markup.indexOf('data-slot="card"');
    const tableStart = markup.indexOf('data-slot="table"');

    expect(toolbarStart).toBeGreaterThan(-1);
    expect(toolbarStart).toBeLessThan(cardStart);
    expect(cardStart).toBeLessThan(tableStart);
    expect(markup).toContain('data-table-panel=""');
    expect(markup).toContain('aria-label="日志筛选"');
    expect(markup).toContain('aria-label="搜索任务、对象或原因"');
    expect(markup).toContain('role="tablist"');
    expect(markup).toContain('aria-label="记录类型"');
    expect(markup.indexOf('aria-label="记录类型"')).toBeLessThan(
      markup.indexOf('aria-label="搜索任务、对象或原因"'),
    );
  });

  it("keeps the long table in an independent scroll region", () => {
    const markup = renderLogsPage();
    const pageContent = openingTag(markup, 'data-slot="page-content"');
    const tableShell = openingTag(markup, 'data-testid="logs-table-shell"');
    const scrollRegion = openingTag(markup, 'data-testid="logs-table-scroll-region"');
    const tableContainer = openingTag(markup, 'data-slot="table-container"');

    expect(pageContent).toContain("overflow-hidden");
    expect(tableShell).toContain("min-h-0");
    expect(tableShell).toContain("flex-1");
    expect(scrollRegion).toContain("min-h-0");
    expect(scrollRegion).toContain("flex-1");
    expect(scrollRegion).toContain("overflow-hidden");
    expect(tableContainer).toContain("h-full");
    expect(tableContainer).toContain("overflow-auto");
    expect(tableContainer).toContain("overscroll-contain");
  });

  it("keeps pagination outside the scroll region without allowing it to shrink", () => {
    const markup = renderLogsPage({
      items: [
        {
          id: "task:1",
          kind: "task",
          occurred_at: "2026-08-27T00:00:00Z",
          title: "inspection-run",
          summary: "巡检完成",
          status: "succeeded",
          actor: null,
          object_label: null,
          source: "run_record",
          source_id: "1",
          related_count: 0,
          details: {},
        },
      ],
      total: 42,
      page: 1,
      page_size: 20,
      counts: {},
      truncated: false,
    });
    const toolbarEnd = markup.indexOf('data-slot="card"');
    const paginationTotal = markup.indexOf(">42</span>");
    const tableEnd = markup.indexOf("</table>");
    const paginationStart = markup.indexOf('data-testid="logs-pagination-region"');
    const paginationRegion = openingTag(markup, 'data-testid="logs-pagination-region"');

    expect(markup.slice(0, toolbarEnd)).not.toContain(">42</span>");
    expect(paginationTotal).toBeGreaterThan(markup.indexOf('data-slot="table"'));
    expect(paginationStart).toBeGreaterThan(tableEnd);
    expect(paginationRegion).toContain("shrink-0");
    expect(markup.match(/>42<\/span>/g)).toHaveLength(1);
  });

  it("preserves stable widths for metadata and action columns", () => {
    const markup = renderLogsPage();

    expect(markup).toMatch(/class="[^"]*w-40[^"]*"[^>]*>时间<\/th>/);
    expect(markup).toMatch(/class="[^"]*w-28[^"]*"[^>]*>类型<\/th>/);
    expect(markup).toMatch(/class="[^"]*w-44[^"]*"[^>]*>对象 \/ 执行人<\/th>/);
    expect(markup).toMatch(/class="[^"]*w-24[^"]*"[^>]*>状态<\/th>/);
    expect(markup).toMatch(/class="[^"]*w-16[^"]*"[^>]*>操作<\/th>/);
    expect(openingTag(markup, 'data-slot="table"')).toContain("min-w-[920px]");
  });
});
