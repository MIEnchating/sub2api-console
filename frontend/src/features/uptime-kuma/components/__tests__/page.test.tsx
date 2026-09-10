import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { KumaConfigPage } from "../kuma-config-page";
import { UptimeKumaPage } from "../uptime-kuma-page";
import { config, monitor, task } from "./fixtures";

function renderPage(
  fetcher: (path: string, init?: RequestInit) => Response | Promise<Response>,
  initialPath = "/uptime-kuma",
): void {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => Promise.resolve(fetcher(input, init))),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const root = createRootRoute();
  const router = createRouter({
    routeTree: root.addChildren([
      createRoute({ getParentRoute: () => root, path: "/uptime-kuma", component: UptimeKumaPage }),
      createRoute({
        getParentRoute: () => root,
        path: "/uptime-kuma/config",
        component: KumaConfigPage,
      }),
    ]),
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Uptime Kuma 页面 API 流程", () => {
  it("配置页主操作固定在页头且不提供管理页互跳入口", async () => {
    renderPage(() => Response.json(config), "/uptime-kuma/config");
    await screen.findByLabelText("API 密钥");
    expect(
      screen.getByRole("button", { name: "验证并保存" }).closest('[data-slot="page-heading"]'),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "断开接入" }).closest('[data-slot="page-heading"]'),
    ).not.toBeNull();
    expect(screen.queryByRole("link", { name: "监控管理" })).not.toBeInTheDocument();
  });
  it("直接打开接入配置页时只加载配置并显示独立表单", async () => {
    const paths: string[] = [];
    renderPage((path) => {
      paths.push(path);
      return Response.json(config);
    }, "/uptime-kuma/config");
    expect(await screen.findByLabelText("API 密钥")).toHaveValue("");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "监控项列表" })).not.toBeInTheDocument();
    expect(paths).toEqual(["/api/uptime-kuma/config"]);
  });

  it("管理页只显示全宽监控表格且没有接入配置互跳入口", async () => {
    renderPage((path) =>
      Response.json(
        path.endsWith("/config") ? config : { config, monitors: [monitor], warning: "" },
      ),
    );
    await screen.findByRole("button", { name: "查看 智谱 详情" });
    expect(screen.queryByLabelText("API 密钥")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "断开接入" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "接入配置" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "监控项列表" })).toHaveClass("flex-1");
    expect(screen.getByRole("button", { name: "删除" }).closest("td")).not.toBeNull();
  });

  it("管理账号验证任务失败时在配置页显示字段错误且保留输入", async () => {
    const user = userEvent.setup({ skipHover: true });
    renderPage((path, init) => {
      if (init?.method === "PUT") return Response.json({ ...task, status: "queued" });
      if (path.startsWith("/api/tasks/"))
        return Response.json({
          ...task,
          status: "failed",
          message: "请检查管理密码",
          result: { error_code: "kuma_auth_failed" },
        });
      return Response.json(config);
    }, "/uptime-kuma/config");
    await screen.findByLabelText("管理密码");
    await user.type(screen.getByLabelText("管理密码"), "test-password");
    await user.click(screen.getByRole("button", { name: "验证并保存" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("请检查管理密码");
    expect(screen.getByLabelText("管理密码")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("管理密码")).toHaveValue("test-password");
  });

  it("远端读取失败时不显示页内错误并禁用新建", async () => {
    renderPage((path) =>
      path.endsWith("/config")
        ? Response.json(config)
        : Response.json(
            { code: "kuma_auth_failed", detail: "登录已失效，请重新配置" },
            { status: 422 },
          ),
    );
    await screen.findByRole("button", { name: "新增监控项" });
    await waitFor(() => expect(screen.getByRole("button", { name: "刷新监控项" })).toBeEnabled());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "接入配置" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "新增监控项" })).toBeDisabled();
  });
  it("配置正在加载时不显示未接入空状态", async () => {
    let resolve: (response: Response) => void = () => undefined;
    renderPage(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("正在读取接入配置");
    expect(screen.queryByRole("link", { name: "配置接入" })).not.toBeInTheDocument();
    resolve(Response.json({ ...config, api_key_configured: false, management_configured: false }));
    expect(await screen.findByText("请在侧栏「Uptime Kuma → 接入配置」中完成接入。")).toBeVisible();
  });
});
