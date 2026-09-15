import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AccountWorkbenchPage } from "../components/account-workbench-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("没有管理目标时默认进入本地导出且不请求线上模板", async () => {
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json({ detail: "尚未配置管理目标" }, { status: 409 }),
  );
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["setup-status"], { initialized: true, target_configured: false });
  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <AccountWorkbenchPage />
      </QueryClientProvider>
    </StrictMode>,
  );
  expect(screen.queryByRole("combobox", { name: "账号操作" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "仅导出 JSON", pressed: true })).toBeVisible();
  expect(fetcher.mock.calls.some(([url]) => String(url).includes("/templates"))).toBe(false);
});
