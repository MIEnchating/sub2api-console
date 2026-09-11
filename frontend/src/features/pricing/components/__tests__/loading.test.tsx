import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RevenueAnalysisPage } from "../revenue-analysis-page";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("收益分析首次读取时显示骨架屏，不使用进度条占位", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RevenueAnalysisPage />
    </QueryClientProvider>,
  );
  const loading = screen.getByRole("status", { name: /正在读取/ });
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});
