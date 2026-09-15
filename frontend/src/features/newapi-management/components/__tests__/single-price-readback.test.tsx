import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";
import { api, type NewAPIRemoteSnapshot } from "@/api";
import { NewAPIManagementPage } from "../newapi-management-page";

let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  client?.clear();
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("单项写入后读回旧价格时提示不一致且不解锁还原", async () => {
  const snapshot: NewAPIRemoteSnapshot = {
    groups: [],
    models: [{ model: "gpt-5", input_ratio: "1", completion_ratio: "8" }],
    unset_models: [],
    tool_prices: [],
    references: [],
    differences: [],
    fetched_at: "2026-09-14T00:00:00Z",
  };
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [
      {
        id: "primary",
        name: "主平台",
        base_url: "https://newapi.example",
        user_id: "1",
        admin_key_configured: true,
        updated_at: "2026-09-14T00:00:00Z",
      },
    ],
    local_groups: [],
    bindings: [],
    sub2api_base_url: "https://sub2api.example",
  });
  vi.spyOn(api, "refreshNewAPIPlatform").mockResolvedValue(snapshot);
  vi.spyOn(api, "saveNewAPIModelPrices").mockResolvedValue(snapshot);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <NewAPIManagementPage view="prices" />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "上调 gpt-5 价格" }));
  fireEvent.click(screen.getByRole("button", { name: "确认上调" }));
  expect(await screen.findByText("gpt-5 已提交，但平台读回价格与目标不一致，请核对")).toBeVisible();
  fireEvent.click(
    within(screen.getByRole("dialog", { name: "gpt-5 写入结果" })).getByRole("button", {
      name: "关闭",
    }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "取消" }));
  expect(await screen.findByRole("button", { name: "还原 gpt-5 价格" })).toBeDisabled();
});
