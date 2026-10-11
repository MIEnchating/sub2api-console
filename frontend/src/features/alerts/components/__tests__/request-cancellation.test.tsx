import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { AlertsPage } from "@/App";
import { api } from "@/api";
import { NotificationQueueStatus } from "../notification-queue-status";

let client: QueryClient | undefined;

afterEach(() => {
  cleanup();
  client?.clear();
  client = undefined;
  vi.unstubAllGlobals();
});

const queues = {
  producer_firing: 0,
  producer_recovered: 0,
  consumer_pending: 0,
  consumer_failed: 0,
  consumer_active: false,
};

const emptyDetails = {
  producer_firing: [],
  producer_recovered: [],
  consumer_pending: [],
  consumer_failed: [],
  consumer_items: [],
};

it("关闭正在读取的队列弹窗会取消请求，重开后旧响应不结束新请求", async () => {
  const user = userEvent.setup();
  const requests: { signal: AbortSignal | null | undefined; resolve: (value: Response) => void }[] =
    [];
  vi.stubGlobal(
    "fetch",
    (_input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((resolve) => requests.push({ signal: init?.signal, resolve })),
  );
  render(<NotificationQueueStatus queues={queues} loadDetails={api.notificationQueue} />);
  await user.click(screen.getByRole("button", { name: "查看队列" }));
  await waitFor(() => expect(requests).toHaveLength(1));
  await user.click(screen.getByRole("button", { name: "关闭" }));
  expect(requests[0].signal?.aborted).toBe(true);

  await user.click(screen.getByRole("button", { name: "查看队列" }));
  await waitFor(() => expect(requests).toHaveLength(2));
  expect(requests[1].signal?.aborted).toBe(false);
  await act(async () => requests[0].resolve(Response.json(emptyDetails)));
  expect(screen.getByRole("status", { name: "正在读取队列内容" })).toBeVisible();
  await act(async () => requests[1].resolve(Response.json(emptyDetails)));
  expect(await screen.findByText("没有匹配的队列内容")).toBeVisible();
});

it("卸载队列入口会取消在途读取", async () => {
  const user = userEvent.setup();
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>(() => {});
  });
  const view = render(
    <NotificationQueueStatus queues={queues} loadDetails={api.notificationQueue} />,
  );
  await user.click(screen.getByRole("button", { name: "查看队列" }));
  view.unmount();
  expect(signal?.aborted).toBe(true);
});

it("离开告警通知页会取消列表和队列概览读取，晚到响应不写缓存", async () => {
  const requests = new Map<
    string,
    { signal: AbortSignal | null | undefined; resolve: (value: Response) => void }
  >();
  vi.stubGlobal(
    "fetch",
    (input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((resolve) =>
        requests.set(String(input), { signal: init?.signal, resolve }),
      ),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["dictionaries", "alert_status"], { items: [] });
  const view = render(
    <QueryClientProvider client={client}>
      <AlertsPage />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(requests.size).toBe(2));
  view.unmount();
  for (const request of requests.values()) expect(request.signal?.aborted).toBe(true);
  await act(async () => {
    requests.get("/api/alerts?limit=200")!.resolve(Response.json([]));
    requests.get("/api/notifications/status")!.resolve(Response.json({ configured: true, queues }));
  });
  expect(client.getQueryData(["alerts"])).toBeUndefined();
  expect(client.getQueryData(["notification-status"])).toBeUndefined();
});
