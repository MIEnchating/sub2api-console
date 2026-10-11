import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { UptimeKumaPage } from "../uptime-kuma-page";
import { KumaResourcesPage } from "../resources-page";
import { KumaConfigPage } from "../kuma-config-page";
import { kumaConfigKey, kumaMonitorsKey, kumaResourcesKey } from "../../constants";
import { config, monitor } from "./fixtures";
import { statusOptions, statusPage } from "./status-page-fixtures";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["monitor-edit", "monitor-confirm", "resource-edit", "resource-confirm"])(
  "%s 打开后更换接入配置，提交被阻止且提示重新打开，不向新实例发送旧操作",
  async (scenario) => {
    const monitorPage = scenario.startsWith("monitor");
    const confirmation = scenario.endsWith("confirm");
    const error = vi.spyOn(toast, "error");
    const writes: string[] = [];
    vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        writes.push(path);
        return new Promise<Response>(() => {});
      }
      if (path.includes("/dictionaries") || path.endsWith("/templates"))
        return Response.json(path.includes("dictionaries") ? { items: [] } : []);
      if (path.endsWith("/10")) return Response.json(statusPage);
      throw new Error(`Unexpected request: ${path}`);
    });
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    client.setQueryData(kumaConfigKey, config);
    client.setQueryData([...kumaMonitorsKey, config.revision], { config, monitors: [monitor] });
    client.setQueryData([...kumaResourcesKey, "status-pages", config.revision], statusOptions);
    render(
      <QueryClientProvider client={client}>
        {monitorPage ? <UptimeKumaPage /> : <KumaResourcesPage kind="status-pages" />}
      </QueryClientProvider>,
    );
    const action = confirmation ? "删除" : "编辑";
    fireEvent.click(screen.getByRole("button", { name: action }));
    if (!confirmation) {
      await screen.findByLabelText(monitorPage ? "监控项名称" : "状态页标题");
    }
    const changed = { ...config, revision: 2, base_url: "https://another.example.test" };
    await act(async () => {
      client.setQueryData([...kumaMonitorsKey, changed.revision], {
        config: changed,
        monitors: [monitor],
      });
      client.setQueryData([...kumaResourcesKey, "status-pages", changed.revision], {
        ...statusOptions,
        config: changed,
      });
      client.setQueryData(kumaConfigKey, changed);
    });
    const dialog = screen.getByRole("dialog");
    await waitFor(() =>
      expect(document.querySelector(`a[href^="${changed.base_url}"]`)).not.toBeNull(),
    );
    fireEvent.click(
      within(dialog).getByRole("button", {
        name: confirmation ? "删除" : /^保存/,
      }),
    );
    await waitFor(() =>
      expect(error).toHaveBeenCalledWith(
        expect.stringMatching(/配置.*更改.*重新/),
        expect.any(Object),
      ),
    );
    expect(writes).toEqual([]);
  },
);

it("断开确认期间接入配置变更时不删除新的接入配置", async () => {
  const writes: string[] = [];
  const error = vi.spyOn(toast, "error");
  vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
    if (init?.method === "DELETE") {
      writes.push(path);
      return new Promise<Response>(() => {});
    }
    throw new Error(`Unexpected request: ${path}`);
  });
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  client.setQueryData(kumaConfigKey, config);
  render(
    <QueryClientProvider client={client}>
      <KumaConfigPage />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "断开接入" }));
  await act(async () => {
    client.setQueryData(kumaConfigKey, {
      ...config,
      revision: 2,
      base_url: "https://another.example.test",
    });
  });
  await waitFor(() =>
    expect(screen.getByLabelText("服务地址")).toHaveValue("https://another.example.test"),
  );
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "断开接入" }));
  await waitFor(() =>
    expect(error).toHaveBeenCalledWith(
      expect.stringMatching(/配置.*更改.*重新/),
      expect.any(Object),
    ),
  );
  expect(writes).toEqual([]);
});
