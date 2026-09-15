import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AlertsPage } from "@/App";
import type { AlertIncident } from "@/api";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("告警后台刷新失败时保留记录及分页，清空操作仍禁用", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ detail: "告警暂时不可用" }, { status: 503 })),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const alert: AlertIncident = {
    incident_key: "incident-1",
    event_type: "account.probe",
    object_kind: "account",
    object_id: "41",
    object_name: "回归测试账号",
    cause_code: "PROBE",
    status: "recovered",
    first_seen_at: "2026-09-01T08:00:00Z",
    last_seen_at: "2026-09-01T08:10:00Z",
    last_error: null,
    delivery_status: "sent",
    delivery_attempts: 1,
    delivered_at: null,
  };
  client.setQueryData(["alerts"], [alert]);
  client.setQueryData(["notification-status"], {
    configured: true,
    queues: {
      producer_firing: 0,
      producer_recovered: 0,
      consumer_pending: 0,
      consumer_failed: 0,
      consumer_active: false,
    },
  });
  client.setQueryData(["dictionaries", "alert_status"], { items: [] });
  render(
    <QueryClientProvider client={client}>
      <AlertsPage />
    </QueryClientProvider>,
  );
  expect(screen.getByText(/回归测试账号/)).toBeVisible();
  expect(screen.getByRole("navigation", { name: "表格分页" })).toBeVisible();
  await act(async () => {
    await client.refetchQueries({ queryKey: ["alerts"], exact: true });
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "清理已结束" })).toBeDisabled());
  expect(screen.getByText(/回归测试账号/)).toBeVisible();
  expect(screen.getByRole("navigation", { name: "表格分页" })).toBeVisible();
  expect(screen.getByRole("button", { name: "清理已结束" })).toBeDisabled();
});
