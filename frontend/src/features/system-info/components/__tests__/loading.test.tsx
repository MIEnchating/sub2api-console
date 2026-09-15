import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { SystemInfoPage } from "../system-info-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("资源首次读取失败后停止骨架并提供重试，重试成功后展示资源指标", async () => {
  let fail = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      if (url.includes("/api/system/metrics"))
        return new Response(
          JSON.stringify(
            fail
              ? { detail: "采样暂不可用" }
              : {
                  cpu: { usage_percent: 20, logical_cores: 4 },
                  memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
                  disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
                },
          ),
          { status: fail ? 503 : 200, headers: { "Content-Type": "application/json" } },
        );
      return new Response("[]", { headers: { "Content-Type": "application/json" } });
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <SystemInfoPage />
    </QueryClientProvider>,
  );
  const resources = screen.getByLabelText("服务器资源占用");
  const retry = await within(resources).findByRole("button", { name: "重新读取" });
  expect(within(resources).queryAllByRole("status", { name: "正在读取资源占用" })).toHaveLength(0);
  fail = false;
  await userEvent.setup().click(retry);
  expect(await within(resources).findByText("CPU 占用")).toBeVisible();
  expect(within(resources).queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
