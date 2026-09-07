import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { AutoInspectionPage } from "@/App";
import type { AutoInspectionStatus } from "@/api";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it.each([true, false])("保存巡检设置期间锁定心跳输入，success=%s 后恢复编辑", async (success) => {
  const user = userEvent.setup();
  const status: AutoInspectionStatus = {
    enabled: false,
    interval_seconds: 15,
    running: false,
    monitoring_configured: true,
    monitoring_enabled: false,
    monitoring_checked_at: null,
    last_run_duration_ms: 0,
    last_summary: {
      channels: 0,
      probed: 0,
      samples: 0,
      fused: 0,
      recovered: 0,
      applied: 0,
      cleaned_up: 0,
      alerts: 0,
    },
    last_run_at: null,
    next_run_at: null,
    last_status: null,
    last_error: null,
    last_task_id: null,
    queue: [],
    heartbeat_history: [],
  };
  let finish!: (value: Response) => void;
  const saving = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") return saving;
      return new Response(JSON.stringify(status));
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  });
  clients.push(client);
  client.setQueryData(["auto-inspection"], status);
  render(
    <QueryClientProvider client={client}>
      <AutoInspectionPage />
    </QueryClientProvider>,
  );

  await user.click(screen.getByRole("button", { name: "保存自动巡检" }));

  const interval = screen.getByRole("spinbutton", { name: "调度心跳周期" });
  await waitFor(() => expect(screen.getByRole("button", { name: "保存自动巡检" })).toBeDisabled());
  expect(interval).toBeDisabled();
  expect(interval).toHaveValue(15);
  await act(async () =>
    finish(
      new Response(
        JSON.stringify(success ? status : { error: { code: "save_failed", message: "保存失败" } }),
        { status: success ? 200 : 500 },
      ),
    ),
  );

  await waitFor(() => expect(interval).toBeEnabled());
  await user.clear(interval);
  await user.type(interval, "30");
  expect(interval).toHaveValue(30);
});
