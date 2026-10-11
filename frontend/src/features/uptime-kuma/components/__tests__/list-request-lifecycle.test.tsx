import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { KumaConfigPage } from "../kuma-config-page";
import { KumaResourcesPage } from "../resources-page";
import { KumaTemplatesPage } from "../templates-page";
import { UptimeKumaPage } from "../uptime-kuma-page";
import {
  kumaConfigKey,
  kumaMonitorsKey,
  kumaResourcesKey,
  kumaTemplatesKey,
} from "../../constants";
import { config, monitor } from "./fixtures";
import { statusOptions } from "./status-page-fixtures";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["config", "monitors", "resources", "templates", "editor-templates"] as const)(
  "%s 的最后一个读取者离开后中止请求，迟到结果不写入缓存且不弹错误",
  async (kind) => {
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    const error = vi.spyOn(toast, "error");
    let signal: AbortSignal | null | undefined;
    let finish!: (response: Response) => void;
    const fetch = vi.fn((_path: string, init?: RequestInit) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    });
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) =>
      path.includes("/dictionaries")
        ? Promise.resolve(Response.json({ items: [] }))
        : fetch(path, init),
    );
    if (kind !== "config") client.setQueryData(kumaConfigKey, config);
    let page = <KumaConfigPage />;
    let key: readonly unknown[] = kumaConfigKey;
    let response: unknown = config;
    if (kind === "monitors" || kind === "editor-templates") {
      page = <UptimeKumaPage />;
      key = [...kumaMonitorsKey, config.revision];
      response = { config, monitors: [monitor] };
    }
    if (kind === "editor-templates") {
      client.setQueryData(key, response);
      key = kumaTemplatesKey;
      response = [];
    }
    if (kind === "resources") {
      page = <KumaResourcesPage kind="status-pages" />;
      key = [...kumaResourcesKey, "status-pages", config.revision];
      response = statusOptions;
    }
    if (kind === "templates") {
      page = <KumaTemplatesPage />;
      key = kumaTemplatesKey;
      response = [];
    }
    const view = render(<QueryClientProvider client={client}>{page}</QueryClientProvider>);
    if (kind === "editor-templates") {
      fireEvent.click(screen.getByRole("button", { name: "新增监控项" }));
    }
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    if (kind === "editor-templates") {
      fireEvent.click(screen.getByRole("button", { name: "取消" }));
    } else {
      view.unmount();
    }
    await act(async () => finish(Response.json(response)));
    expect(signal?.aborted).toBe(true);
    expect(client.getQueryData(key)).toBeUndefined();
    expect(error).not.toHaveBeenCalled();
  },
);

it("共享接入配置仍有页面使用时保留请求，最后一个页面离开才中止", async () => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  let signal: AbortSignal | null | undefined;
  const fetch = vi.fn((_path: string, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>(() => {});
  });
  vi.stubGlobal("fetch", fetch);
  const first = render(
    <QueryClientProvider client={client}>
      <KumaConfigPage />
    </QueryClientProvider>,
  );
  const second = render(
    <QueryClientProvider client={client}>
      <UptimeKumaPage />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  first.unmount();
  expect(signal?.aborted).toBe(false);
  second.unmount();
  expect(signal?.aborted).toBe(true);
});

it("配置刷新失败保留监控列表但禁用编辑，重新读取成功后恢复操作", async () => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(kumaConfigKey, config);
  client.setQueryData([...kumaMonitorsKey, config.revision], { config, monitors: [monitor] });
  let failed = true;
  vi.stubGlobal("fetch", async (path: string) => {
    if (path.endsWith("/config")) {
      return failed
        ? Response.json({ detail: "接入配置暂时不可用" }, { status: 503 })
        : Response.json(config);
    }
    if (path.endsWith("/dictionaries")) return Response.json({ items: [] });
    return Response.json({ config, monitors: [monitor] });
  });
  render(
    <QueryClientProvider client={client}>
      <UptimeKumaPage />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "刷新监控项" }));
  await waitFor(() => expect(client.getQueryState(kumaConfigKey)?.status).toBe("error"));
  expect(screen.getByRole("cell", { name: "智谱" })).toBeVisible();
  expect(screen.getByRole("button", { name: "新增监控项" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "编辑" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "暂停" })).toBeDisabled();
  failed = false;
  fireEvent.click(screen.getByRole("button", { name: "刷新监控项" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "编辑" })).toBeEnabled());
});

it.each(["config", "monitors"] as const)(
  "监控页首次 %s 读取失败提供局部重新读取入口，重试成功后显示列表",
  async (kind) => {
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    if (kind === "monitors") client.setQueryData(kumaConfigKey, config);
    let failed = true;
    vi.stubGlobal("fetch", async (path: string) => {
      if (failed) return Response.json({ detail: "列表读取暂时不可用" }, { status: 503 });
      return Response.json(path.endsWith("/config") ? config : { config, monitors: [monitor] });
    });
    render(
      <QueryClientProvider client={client}>
        <UptimeKumaPage />
      </QueryClientProvider>,
    );
    const retry = await screen.findByRole("button", { name: "重新读取" });
    failed = false;
    fireEvent.click(retry);
    expect(await screen.findByRole("cell", { name: "智谱" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  },
);
