import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { UnifiedLogPage } from "@/api";
import { LogsCenterPage } from "../components/logs-center-page";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function response(search: string): UnifiedLogPage {
  return {
    items: [
      {
        id: "event:" + search,
        kind: "event",
        occurred_at: "2026-10-09T00:00:00Z",
        title: "搜索结果",
        summary: search || "初始日志",
        status: "succeeded",
        actor: null,
        object_label: null,
        source: "runtime_event",
        source_id: "1",
        related_count: 0,
        details: {},
      },
    ],
    total: 1,
    page: 1,
    page_size: 20,
    counts: { event: 1 },
    truncated: false,
  };
}

async function mountLogs(): Promise<void> {
  vi.stubGlobal("scrollTo", vi.fn());
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const root = createRootRoute();
  const route = createRoute({
    getParentRoute: () => root,
    path: "/logs",
    component: () => (
      <QueryClientProvider client={client}>
        <LogsCenterPage />
      </QueryClientProvider>
    ),
  });
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ["/logs"] }),
  });
  await router.load();
  render(<RouterProvider router={router} />);
  await screen.findByText("初始日志");
}

it("连续输入关键词时立即更新输入框，停顿后只请求最后一个关键词", async () => {
  const searches: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname !== "/api/logs") return Response.json({ items: [] });
      const search = url.searchParams.get("search") ?? "";
      searches.push(search);
      return Response.json(response(search));
    }),
  );
  await mountLogs();
  vi.useFakeTimers();
  const input = screen.getByRole("textbox", { name: "搜索任务、对象或原因" });
  fireEvent.change(input, { target: { value: "a" } });
  await act(async () => {});
  fireEvent.change(input, { target: { value: "alpha" } });
  await act(async () => {});
  expect(input).toHaveValue("alpha");
  expect(searches).toEqual([""]);
  expect(screen.getByText("初始日志")).toBeInTheDocument();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  expect(searches).toEqual(["", "alpha"]);
});

it("关键词变化时取消旧请求，迟到结果不能覆盖当前搜索", async () => {
  let staleSignal: AbortSignal | null | undefined;
  let completeStale!: (value: Response) => void;
  const stale = new Promise<Response>((resolve) => {
    completeStale = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname !== "/api/logs") return Response.json({ items: [] });
      const search = url.searchParams.get("search") ?? "";
      if (search === "slow") {
        staleSignal = init?.signal;
        return stale;
      }
      return Response.json(response(search));
    }),
  );
  await mountLogs();
  vi.useFakeTimers();
  const input = screen.getByRole("textbox", { name: "搜索任务、对象或原因" });
  fireEvent.change(input, { target: { value: "slow" } });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  fireEvent.change(input, { target: { value: "latest" } });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  expect(staleSignal?.aborted).toBe(true);
  await act(async () => {
    completeStale(Response.json(response("slow")));
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(screen.queryByText("slow")).not.toBeInTheDocument();
  expect(screen.getByText("latest")).toBeInTheDocument();
});
