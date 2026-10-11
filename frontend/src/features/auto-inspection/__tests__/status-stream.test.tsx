import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import App from "@/app-shell";
import type { AutoInspectionStatus } from "@/api";

const status: AutoInspectionStatus = {
  enabled: false,
  running: false,
  interval_seconds: 60,
  monitoring_configured: false,
  monitoring_enabled: false,
  monitoring_checked_at: null,
  last_run_at: null,
  next_run_at: null,
  last_status: null,
  last_error: null,
  last_task_id: null,
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
  queue: [],
  heartbeat_history: [],
};

class StatusStream extends EventTarget {
  static current: StatusStream;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  constructor() {
    super();
    StatusStream.current = this;
  }
  send(data: string): void {
    this.dispatchEvent(new MessageEvent("status", { data }));
  }
}

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  localStorage.clear();
});

async function setup(): Promise<ReturnType<typeof vi.fn>> {
  vi.useFakeTimers();
  vi.stubGlobal("EventSource", StatusStream);
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal(
    "matchMedia",
    vi.fn((media: string) => ({
      matches: false,
      media,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
  const readStatus = vi.fn(async () => Response.json(status));
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      url === "/api/inspection/automation" ? readStatus() : Response.json({ open_alerts: 0 }),
    ),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["setup-status"], { initialized: true, configuration_errors: [] });
  client.setQueryData(["session"], { authenticated: true, username: "fixture" });
  client.setQueryData(["navigation-preferences"], { hidden_item_ids: [], version: "1" });
  client.setQueryData(["auto-inspection"], status);
  client.setQueryData(["overview"], { open_alerts: 0 });
  const root = createRootRoute({ component: App });
  const router = createRouter({
    routeTree: root.addChildren([createRoute({ getParentRoute: () => root, path: "/" })]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  await act(async () => {
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    await router.load();
  });
  return readStatus;
}

it("事件流仅建立连接但未收到状态时继续轮询，收到有效状态后停止轮询", async () => {
  const readStatus = await setup();
  await act(async () => {
    StatusStream.current.onopen?.();
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15_000);
  });
  expect(readStatus).toHaveBeenCalledTimes(1);
  await act(async () => {
    StatusStream.current.send(JSON.stringify({ ...status, enabled: true }));
  });
  expect(screen.getByText("自动巡检中")).toBeVisible();
  readStatus.mockClear();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(30_000);
  });
  expect(readStatus).not.toHaveBeenCalled();
});

it.each([
  { label: "损坏 JSON", payload: "{invalid" },
  { label: "空状态", payload: "null" },
  { label: "错误的开关类型", payload: JSON.stringify({ ...status, enabled: "false" }) },
  {
    label: "错误的队列项",
    payload: JSON.stringify({ ...status, queue: [{ label: "incomplete" }] }),
  },
])("事件流收到$label 时保留当前显示并恢复 HTTP 轮询", async (scenario) => {
  const readStatus = await setup();
  await act(async () => {
    StatusStream.current.onopen?.();
    StatusStream.current.send(JSON.stringify({ ...status, enabled: true }));
  });
  expect(screen.getByText("自动巡检中")).toBeVisible();
  await act(async () => {
    StatusStream.current.send(scenario.payload);
  });
  expect(screen.getByText("自动巡检中")).toBeVisible();
  await act(async () => {
    // Include the query observer notification queued by the polling tick.
    await vi.advanceTimersByTimeAsync(15_001);
  });
  expect(readStatus).toHaveBeenCalledTimes(1);
  expect(screen.getByText("自动巡检已暂停")).toBeVisible();
  expect(StatusStream.current.close).not.toHaveBeenCalled();
  cleanup();
  expect(StatusStream.current.close).toHaveBeenCalledOnce();
});

it("HTTP 轮询响应晚于有效推送到达时不覆盖推送的新状态", async () => {
  const readStatus = await setup();
  let finish!: (response: Response) => void;
  readStatus.mockImplementationOnce(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15_000);
  });
  expect(readStatus).toHaveBeenCalledOnce();
  await act(async () => {
    StatusStream.current.send(JSON.stringify({ ...status, enabled: true }));
  });
  expect(screen.getByText("自动巡检中")).toBeVisible();
  await act(async () => {
    finish(Response.json(status));
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(screen.getByText("自动巡检中")).toBeVisible();
});

it("Go 空切片推送保留为空列表，连接中断后恢复轮询", async () => {
  const readStatus = await setup();
  await act(async () => {
    StatusStream.current.send(
      JSON.stringify({ ...status, enabled: true, queue: null, heartbeat_history: null }),
    );
  });
  expect(client.getQueryData<AutoInspectionStatus>(["auto-inspection"])?.queue).toEqual([]);
  expect(client.getQueryData<AutoInspectionStatus>(["auto-inspection"])?.heartbeat_history).toEqual(
    [],
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(30_000);
  });
  expect(readStatus).not.toHaveBeenCalled();
  await act(async () => {
    StatusStream.current.onerror?.();
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15_001);
  });
  expect(readStatus).toHaveBeenCalledOnce();
  expect(screen.getByText("自动巡检已暂停")).toBeVisible();
});
