import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchExports } from "../components/workbench-exports";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("从处理记录导出时只提交原任务ID并先展示本批范围", async () => {
  const requests: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        path: String(url),
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      return Response.json({
        id: "batch-export-preview",
        target: "https://site.example.test",
        expires_at: new Date(Date.now() + 600000).toISOString(),
        items: [{ account_id: "41", name: "本批账号", revision: "account-version" }],
        revision: "preview-version",
      });
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchExports sourceTaskId="original-batch" />
    </QueryClientProvider>,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "导出本批结果" }));
  await screen.findByRole("list", { name: "导出账号范围" });
  expect(requests).toEqual([
    { path: "/api/account-workbench/exports/preview", body: { source_task_id: "original-batch" } },
  ]);
  expect(screen.getByText("本批账号（ID 41）")).toBeVisible();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "确认导出 1 个账号" })).toBeEnabled(),
  );
});
