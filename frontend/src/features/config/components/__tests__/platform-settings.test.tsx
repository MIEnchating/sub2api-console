import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import { ConfigPage } from "@/App";
import type { ConfigTab } from "../../constants";

let client: QueryClient;
beforeAll(async () => {
  // This test covers tab requests; load the real lazy modules before timing UI queries.
  await Promise.all([
    import("@/features/newapi-management/components/newapi-management-page"),
    import("@/features/uptime-kuma/components/kuma-config-page"),
  ]);
});
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function Settings() {
  const [tab, setTab] = useState<ConfigTab>("newapi");
  return <ConfigPage activeTab={tab} onTabChange={setTab} />;
}
it("系统设置内切换平台分类只读取对应配置，保留唯一页头并展示必填说明", async () => {
  const paths: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      paths.push(path);
      if (path.includes("/api/newapi"))
        return Response.json({ platforms: [], local_groups: [], bindings: [] });
      if (path.includes("uptime-kuma/config"))
        return Response.json({
          base_url: "",
          username: "",
          revision: 0,
          api_key_configured: false,
          management_configured: false,
        });
      return Response.json({}, { status: 503 });
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("尚未添加 New API 平台配置")).toBeVisible();
  expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  expect(screen.getByRole("tab", { name: "New API 平台" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  expect(paths.some((path) => path.includes("uptime-kuma"))).toBe(false);
  await userEvent.setup().click(screen.getByRole("tab", { name: "监控平台" }));
  expect(await screen.findByLabelText("服务地址")).toHaveAttribute("aria-required", "true");
  expect(screen.getByLabelText("API 密钥")).toHaveAttribute("aria-required", "true");
  expect(screen.getByLabelText("管理账号（可选）")).toHaveAccessibleDescription(/选填/);
  expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  expect(
    paths.every((path) => path.includes("/api/newapi") || path.includes("uptime-kuma/config")),
  ).toBe(true);
});
