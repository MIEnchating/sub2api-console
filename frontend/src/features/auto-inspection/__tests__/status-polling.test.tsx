import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { SchedulerHeaderControls, SchedulerSidebarStatus } from "../../../app-shell";
import type { AutoInspectionStatus } from "../../../api";

const pausedStatus = {
  enabled: false,
  running: false,
  monitoring_configured: false,
  last_run_at: null,
} as AutoInspectionStatus;

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("实时状态连接后停止轮询，断开后恢复轮询并继续显示推送状态", async () => {
  const fetchStatus = vi.fn(async () => Response.json(pausedStatus));
  vi.stubGlobal("fetch", fetchStatus);
  vi.useFakeTimers();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["auto-inspection"], pausedStatus);

  const controls = (connected: boolean) => (
    <QueryClientProvider client={client}>
      <SchedulerHeaderControls realtimeConnected={connected} />
      <SchedulerSidebarStatus />
    </QueryClientProvider>
  );
  const view = render(controls(true));

  await act(async () => {
    client.setQueryData(["auto-inspection"], { ...pausedStatus, enabled: true });
    await vi.advanceTimersByTimeAsync(30_000);
  });
  expect(screen.getByText("自动巡检中")).toBeInTheDocument();
  expect(fetchStatus).not.toHaveBeenCalled();

  view.rerender(controls(false));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15_000);
  });
  expect(fetchStatus).toHaveBeenCalledTimes(1);

  view.rerender(controls(true));
  fetchStatus.mockClear();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(30_000);
  });
  expect(fetchStatus).not.toHaveBeenCalled();
  client.clear();
});

it("侧栏在路由切换后挂载不会创建第二个状态轮询定时器", async () => {
  const fetchStatus = vi.fn(async () => Response.json(pausedStatus));
  vi.stubGlobal("fetch", fetchStatus);
  vi.useFakeTimers();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["auto-inspection"], pausedStatus);
  const view = render(
    <QueryClientProvider client={client}>
      <SchedulerHeaderControls realtimeConnected={false} />
    </QueryClientProvider>,
  );

  await act(async () => {
    await vi.advanceTimersByTimeAsync(5_000);
  });
  view.rerender(
    <QueryClientProvider client={client}>
      <SchedulerHeaderControls realtimeConnected={false} />
      <SchedulerSidebarStatus />
    </QueryClientProvider>,
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15_000);
  });
  expect(fetchStatus).toHaveBeenCalledTimes(1);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(5_000);
  });
  expect(fetchStatus).toHaveBeenCalledTimes(1);
  client.clear();
});
