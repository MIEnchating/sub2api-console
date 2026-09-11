import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import App from "../App";
import { createConsoleQueryClient } from "../lib/query-client";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  localStorage.clear();
  vi.unstubAllGlobals();
});

it.each(["初始化", "登录状态"])(
  "%s查询失败时只显示悬浮错误，重新连接后可进入登录页",
  async (phase) => {
    vi.stubGlobal("scrollTo", vi.fn());
    let failed = true;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        if (failed && (phase === "初始化" || !path.includes("setup"))) {
          return Response.json({ detail: "控制台连接中断", code: "unavailable" }, { status: 503 });
        }
        return Response.json(
          path.includes("setup") ? { initialized: true } : { authenticated: false },
        );
      }),
    );
    client = createConsoleQueryClient();
    client.setDefaultOptions({ queries: { retry: false } });
    const root = createRootRoute({ component: App });
    const router = createRouter({
      routeTree: root.addChildren([createRoute({ getParentRoute: () => root, path: "/" })]),
      history: createMemoryHistory({ initialEntries: ["/"] }),
    });
    const view = render(
      <QueryClientProvider client={client}>
        <Toaster />
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("控制台连接中断")).toBeVisible();
    expect(screen.getAllByText("控制台连接中断")).toHaveLength(1);
    expect(view.container.querySelectorAll("[data-sonner-toast]")).toHaveLength(1);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    failed = false;
    await userEvent
      .setup({ skipHover: true })
      .click(screen.getByRole("button", { name: "重新连接" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "登录" })).toBeVisible());
  },
);
