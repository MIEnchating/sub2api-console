import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast, Toaster } from "sonner";
import { afterEach, expect, it, vi } from "vitest";

import { UpstreamsPage } from "../App";
import type { UpstreamSummary } from "../api";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

it("上游列表刷新失败时保留已加载行和选择，刷新成功后更新列表", async () => {
  const upstream: UpstreamSummary["hosts"][number] = {
    upstream_id: "upstream-1",
    host: "upstream.example.test",
    hosts: ["upstream.example.test"],
    base_url: "https://upstream.example.test",
    name: "已加载上游",
    upstream_type: "sub2api",
    account_count: 1,
    group_count: 1,
    auth_status: "已鉴权",
    raw_balance: "10",
    balance: "10",
    recharge_rate: "1",
    balance_status: "available",
    checked_at: null,
  };
  let unavailable = true;
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal("fetch", async () => {
    if (unavailable) return Response.json({ detail: "上游列表暂时不可用" }, { status: 503 });
    return Response.json({ hosts: [] });
  });
  client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["upstreams"], { hosts: [upstream] });
  client.setQueryData(["config"], { mode: "监控模式" });
  const root = createRootRoute();
  const route = createRoute({ getParentRoute: () => root, path: "/", component: UpstreamsPage });
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory(),
  });
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "选择上游 已加载上游" }));
  await user.click(screen.getByRole("button", { name: "刷新上游列表" }));
  await screen.findByText("上游列表暂时不可用");

  expect(screen.getByRole("checkbox", { name: "选择上游 已加载上游" })).toBeChecked();
  expect(screen.getByText("已加载上游")).toBeVisible();

  unavailable = false;
  await user.click(screen.getByRole("button", { name: "刷新上游列表" }));
  expect(await screen.findByText("当前业务库没有上游 Host")).toBeVisible();
  expect(screen.queryByRole("checkbox", { name: "选择上游 已加载上游" })).not.toBeInTheDocument();
});
