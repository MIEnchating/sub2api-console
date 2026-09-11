import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import App from "../App";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  localStorage.clear();
  vi.unstubAllGlobals();
});
it.each(["初始化", "会话"])("%s读取中显示紧凑启动反馈，隐藏登录表单和进度条", async (phase) => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  vi.stubGlobal("scrollTo", vi.fn());
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  if (phase === "会话") client.setQueryData(["setup-status"], { initialized: true });
  const root = createRootRoute({ component: App });
  const router = createRouter({
    routeTree: root.addChildren([createRoute({ getParentRoute: () => root, path: "/" })]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  const label = phase === "初始化" ? "正在读取初始化状态…" : "正在验证登录状态…";
  const loading = await screen.findByRole("status", { name: label });
  expect(loading).toHaveTextContent("Sub2API Console");
  expect(loading.querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "登录" })).not.toBeInTheDocument();
});
