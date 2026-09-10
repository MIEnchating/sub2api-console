import { KumaResourcesPage } from "../resources-page";
import { KumaTemplatesPage } from "../templates-page";
import { KumaConfigPage } from "../kuma-config-page";
import { UptimeKumaPage } from "../uptime-kuma-page";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { config } from "./fixtures";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

describe("Uptime Kuma 查询错误提示", () => {
  it.each([
    { name: "功能模板", configFails: false },
    { name: "状态页", configFails: false },
    { name: "监控列表", configFails: false },
    { name: "监控接入配置", configFails: true },
    { name: "接入配置", configFails: true },
  ])("$name 请求失败时仅显示一条悬浮提示，页面可刷新重试", async (scenario) => {
    let page = <UptimeKumaPage />;
    if (scenario.name === "接入配置") page = <KumaConfigPage />;
    if (scenario.name === "功能模板") page = <KumaTemplatesPage />;
    if (scenario.name === "状态页") page = <KumaResourcesPage kind="status-pages" />;
    vi.stubGlobal("PointerEvent", MouseEvent);
    let failed = true;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        if (failed && (scenario.configFails || !path.endsWith("/config"))) {
          return Response.json({ detail: "请求失败（404）", code: "not_found" }, { status: 404 });
        }
        if (path.endsWith("/templates")) return Response.json([]);
        return Response.json(
          path.endsWith("/config") ? config : { config, monitors: [], items: [] },
        );
      }),
    );
    client = createConsoleQueryClient();
    client.setDefaultOptions({ queries: { retry: false } });
    const view = render(
      <QueryClientProvider client={client}>
        <Toaster />
        <main>{page}</main>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("请求失败（404）")).toBeVisible();
    await waitFor(() =>
      expect(view.container.querySelector("main")).not.toHaveTextContent(/失败|重试/),
    );
    expect(view.container.querySelectorAll("[data-sonner-toast]")).toHaveLength(1);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    if (scenario.name === "接入配置") {
      expect(screen.getByRole("button", { name: "验证并保存" })).toBeDisabled();
    } else if (scenario.name === "监控列表") {
      expect(screen.getByRole("button", { name: "新增监控项" })).toBeDisabled();
    }
    failed = false;
    fireEvent.click(screen.getByRole("button", { name: /刷新/ }));
    await waitFor(() =>
      expect(
        client
          .getQueryCache()
          .getAll()
          .filter((query) => query.isActive())
          .every((query) => query.state.status === "success"),
      ).toBe(true),
    );
  });
});
