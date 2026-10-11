import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import App from "@/app-shell";
import { api } from "@/api";
import { sessionExpiredEvent } from "@/lib/session-auth";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  localStorage.clear();
  vi.unstubAllGlobals();
});

function renderApp(authenticated: boolean): void {
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["setup-status"], { initialized: true });
  client.setQueryData(["session"], { authenticated, username: authenticated ? "operator" : null });
  const root = createRootRoute({ component: App });
  const router = createRouter({
    routeTree: root.addChildren([
      createRoute({ getParentRoute: () => root, path: "/", component: () => <p>已登录内容</p> }),
    ]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

function fallbackResponse(path: string): Response {
  if (path.includes("/inspection/automation"))
    return Response.json({
      enabled: false,
      running: false,
      traffic_collection: { enabled: false },
    });
  return Response.json({ items: [], hidden_item_ids: [] });
}

async function submitLogin(): Promise<void> {
  fireEvent.change(await screen.findByRole("textbox", { name: "账号" }), {
    target: { value: "operator" },
  });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), {
    target: { value: "isolated-password" },
  });
  fireEvent.click(screen.getByRole("button", { name: "登录" }));
}

it("登录成功直接使用已确认会话，不额外等待会话读取", async () => {
  let reads = 0;
  vi.stubGlobal("fetch", (path: string) => {
    if (path.endsWith("/auth/login"))
      return Promise.resolve(Response.json({ authenticated: true, username: "operator" }));
    if (path.endsWith("/auth/session")) {
      reads++;
      return new Promise<Response>(() => {});
    }
    return Promise.resolve(fallbackResponse(path));
  });
  renderApp(false);
  await submitLogin();
  expect(await screen.findByText("已登录内容")).toBeVisible();
  expect(reads).toBe(0);
});

it("登录返回未认证结果时保留账号密码并允许重试", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ authenticated: false, username: null }));
  renderApp(false);
  await submitLogin();
  await waitFor(() => expect(screen.getByRole("button", { name: "登录" })).toBeEnabled());
  expect(screen.getByRole("textbox", { name: "账号" })).toHaveValue("operator");
  expect(screen.getByLabelText("密码", { exact: true })).toHaveValue("isolated-password");
  expect(screen.queryByText("已登录内容")).not.toBeInTheDocument();
});

it("退出等待期间禁用退出按钮，失败恢复入口且保留当前会话", async () => {
  let finish!: (response: Response) => void;
  let exits = 0;
  vi.stubGlobal("fetch", (path: string) => {
    if (path.endsWith("/auth/logout")) {
      exits++;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }
    return Promise.resolve(fallbackResponse(path));
  });
  renderApp(true);
  const button = await screen.findByRole("button", { name: "退出登录" });
  fireEvent.click(button);
  await waitFor(() => expect(finish).toBeDefined());
  expect(button).toBeDisabled();
  fireEvent.click(button);
  expect(exits).toBe(1);
  await act(async () => finish(Response.json({ detail: "注销失败" }, { status: 503 })));
  await waitFor(() => expect(button).toBeEnabled());
  expect(screen.getByText("已登录内容")).toBeVisible();
});

it.each([200, 401])(
  "重新登录后丢弃旧会话迟到的 %s 响应，保持新会话和空业务缓存",
  async (status) => {
    let finish!: (response: Response) => void;
    vi.stubGlobal("fetch", (path: string) => {
      if (path.endsWith("/templates/old-session"))
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      if (path.endsWith("/auth/login") || path.endsWith("/auth/session"))
        return Promise.resolve(Response.json({ authenticated: true, username: "operator" }));
      return Promise.resolve(fallbackResponse(path));
    });
    renderApp(true);
    await screen.findByText("已登录内容");
    const oldRead = api
      .kumaTemplate("old-session")
      .then((value) => {
        client.setQueryData(["old-result"], value);
      })
      .catch((error: unknown) => error);
    act(() => window.dispatchEvent(new Event(sessionExpiredEvent)));
    await submitLogin();
    await screen.findByText("已登录内容");
    await act(async () => {
      finish(Response.json({ detail: "旧会话结果", account_count: 9 }, { status }));
      await oldRead;
    });
    expect(client.getQueryData(["session"])).toMatchObject({ authenticated: true });
    expect(client.getQueryData(["old-result"])).toBeUndefined();
    expect(screen.getByText("已登录内容")).toBeVisible();
  },
);
