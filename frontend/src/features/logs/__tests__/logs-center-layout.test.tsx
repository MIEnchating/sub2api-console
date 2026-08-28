import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

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

describe("LogsCenterPage layout", () => {
  it("places the log filter toolbar above the table card", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    queryClient.setQueryData(["logs", "all", "all", "all", "all", "", "", 1, 20], {
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      counts: {},
      truncated: false,
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <LogsCenterPage />
      </QueryClientProvider>,
    );
    const toolbarStart = markup.indexOf('data-slot="table-filter-toolbar"');
    const cardStart = markup.indexOf('data-slot="card"');
    const tableStart = markup.indexOf('data-slot="table"');

    expect(toolbarStart).toBeGreaterThan(-1);
    expect(toolbarStart).toBeLessThan(cardStart);
    expect(cardStart).toBeLessThan(tableStart);
    expect(markup).toContain('aria-label="搜索日志"');
  });

  it("shows the total only in the pagination below the table", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    queryClient.setQueryData(["logs", "all", "all", "all", "all", "", "", 1, 20], {
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
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <LogsCenterPage />
      </QueryClientProvider>,
    );
    const toolbarEnd = markup.indexOf('data-slot="card"');
    const paginationTotal = markup.indexOf(">42</span>");

    expect(markup.slice(0, toolbarEnd)).not.toContain(">42</span>");
    expect(paginationTotal).toBeGreaterThan(markup.indexOf('data-slot="table"'));
    expect(markup.match(/>42<\/span>/g)).toHaveLength(1);
  });
});
