import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast, Toaster } from "sonner";
import { afterEach, expect, it, vi } from "vitest";

import { OnboardingPage } from "../App";
import { api } from "../api";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

function renderOnboarding(): void {
  const root = createRootRoute();
  const route = createRoute({
    getParentRoute: () => root,
    path: "/onboarding",
    component: OnboardingPage,
  });
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ["/onboarding"] }),
  });
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

it("已有分组的后台刷新失败时保留开户表单和已输入名称", async () => {
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal("fetch", async () =>
    Response.json({ detail: "本地分组暂时不可用" }, { status: 503 }),
  );
  client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["groups"], []);
  renderOnboarding();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "名称" }), "待添加上游");

  await act(async () => {
    await expect(
      client.fetchQuery({ queryKey: ["groups"], queryFn: api.groups }),
    ).rejects.toThrow();
  });
  await screen.findByText("本地分组暂时不可用");

  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue("待添加上游");
  expect(screen.getByRole("textbox", { name: "上游地址" })).toBeEnabled();
});

it("首次读取分组失败时提供重试入口并在恢复后显示开户表单", async () => {
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal("fetch", async () =>
    Response.json({ detail: "本地分组暂时不可用" }, { status: 503 }),
  );
  client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  await expect(client.fetchQuery({ queryKey: ["groups"], queryFn: api.groups })).rejects.toThrow();
  renderOnboarding();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("textbox", { name: "名称" })).not.toBeInTheDocument();

  vi.stubGlobal("fetch", async () => Response.json([]));
  await userEvent.setup().click(retry);

  expect(await screen.findByRole("textbox", { name: "名称" })).toBeEnabled();
});
