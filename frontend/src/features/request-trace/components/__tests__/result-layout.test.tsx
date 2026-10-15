import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { SystemLogSearchPanel } from "../system-log-search-panel";

it("警告日志没有 HTTP 状态时展示真实级别，不标为已完成且长 Host 可换行", async () => {
  const host = "long-host-".repeat(30);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      Response.json({
        items: [
          {
            id: 1,
            request_id: "req-warning",
            summary: "upstream retry",
            source: "system-log",
            payload: { level: "warn", host },
          },
        ],
        total: 1,
        page: 1,
        page_size: 20,
      }),
    ),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <SystemLogSearchPanel />
    </QueryClientProvider>,
  );
  try {
    fireEvent.click(screen.getByRole("button", { name: "查询" }));
    await screen.findByText("req-warning");
    expect(screen.getByText("警告（warn）")).toBeVisible();
    expect(screen.queryByText("已完成")).not.toBeInTheDocument();
    expect(screen.getByText(`Host：${host}`)).toHaveClass("break-all", "min-w-0");
  } finally {
    view.unmount();
    client.clear();
    vi.unstubAllGlobals();
  }
});
