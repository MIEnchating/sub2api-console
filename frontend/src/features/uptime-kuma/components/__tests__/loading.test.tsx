import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { KumaResourcesPage } from "../resources-page";
import { KumaTemplatesPage } from "../templates-page";
import { KumaConfigPage } from "../kuma-config-page";
import { UptimeKumaPage } from "../uptime-kuma-page";
import { kumaConfigKey, kumaTemplatesKey } from "../../constants";
import { config } from "./fixtures";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it.each([
  { page: "monitors", configured: false },
  { page: "config", configured: false },
  { page: "templates", configured: false },
  { page: "monitors", configured: true },
  { page: "status-pages", configured: true },
] as const)(
  "$page 主页面 configured=$configured 首次读取时展示骨架屏，不显示进度条或空列表",
  (scenario) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    if (scenario.configured) client.setQueryData(kumaConfigKey, config);
    let page = <UptimeKumaPage />;
    if (scenario.page === "config") page = <KumaConfigPage />;
    if (scenario.page === "templates") page = <KumaTemplatesPage />;
    if (scenario.page === "status-pages") {
      page = <KumaResourcesPage kind={scenario.page} />;
    }
    render(<QueryClientProvider client={client}>{page}</QueryClientProvider>);
    const loading = screen.getByRole("status", { name: /正在读取/ });
    expect(loading).toHaveAttribute("aria-busy", "true");
    expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.queryByText("0%")).not.toBeInTheDocument();
    expect(screen.queryByText("暂无记录")).not.toBeInTheDocument();
  },
);

it("已有模板缓存时后台刷新保留列表，不重新显示主页面骨架", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(kumaConfigKey, config);
  client.setQueryData(kumaTemplatesKey, [
    {
      id: "a".repeat(48),
      revision: 1,
      name: "JSON 模板",
      method: "POST",
      auth_method: "none",
      headers_configured: false,
      body_configured: false,
      auth_configured: false,
    },
  ]);
  render(
    <QueryClientProvider client={client}>
      <KumaTemplatesPage />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("cell", { name: "JSON 模板" })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "刷新" }));
  expect(screen.getByRole("cell", { name: "JSON 模板" })).toBeVisible();
  expect(screen.queryByRole("status", { name: /正在读取/ })).not.toBeInTheDocument();
});
