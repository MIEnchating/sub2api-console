import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchMaintenancePanel } from "../components/workbench-maintenance";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("重新打开维护页仍显示后端下次检查时间和最近一次处理结果", async () => {
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
  client.setQueryData(workbenchKeys.maintenance, {
    enabled: true,
    interval_minutes: 5,
    cooldown_minutes: 0,
    group_ids: [],
    model: "gpt-5.6-sol",
    check_after_repair: true,
    revision: 1,
    next_run_at: "2026-09-15T12:05:00Z",
    last_run_at: "2026-09-15T12:00:00Z",
    last_task_id: "maintenance-previous",
  });
  client.setQueryData(["groups"], []);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        id: "maintenance-previous",
        status: "partial",
        operation: "account-workbench-maintenance",
        message: "检查完成",
        result: {
          items: [{ name: "需要恢复的账号", index: 0, status: "review", message: "授权已失效" }],
        },
      }),
    ),
  );
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMaintenancePanel />
    </QueryClientProvider>,
  );
  expect(screen.getByText(/下次检查/)).toHaveTextContent("2026-09-15T12:05:00Z");
  expect(await screen.findByText("需要恢复的账号")).toBeVisible();
  expect(screen.getByText("授权已失效")).toBeVisible();
  expect(screen.getByRole("spinbutton", { name: "修复冷却时间（分钟）" })).toHaveAttribute(
    "min",
    "0",
  );
});
