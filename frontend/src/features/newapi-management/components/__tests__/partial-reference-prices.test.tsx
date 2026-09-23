import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";

import { api, type NewAPIRemoteSnapshot } from "@/api";
import { NewAPIModelPrices } from "../model-prices";
import { NewAPIManagementPage } from "../newapi-management-page";

const syncError = "模型广场未返回该模型的完整参考价，请检查模型可见性后刷新";
const completePrice = {
  model: "grok-4.5",
  input_price: "0.000002",
  output_price: "0.000006",
  model_ratio: "1",
  completion_ratio: "3",
};
const incompletePrice = {
  ...completePrice,
  model: "private-model",
  billing_expr: 'tier("cached", p * 2 + c * 6)',
  sync_error: syncError,
};
const catalog = { models: [completePrice, incompletePrice], stale: false };
const snapshot: NewAPIRemoteSnapshot = {
  groups: [],
  models: catalog.models.map((price) => ({
    model: price.model,
    input_ratio: "2",
    completion_ratio: "4",
  })),
  unset_models: [],
  tool_prices: [],
  references: [],
  differences: [],
  fetched_at: "2026-09-23T00:00:00Z",
};
let client: QueryClient | undefined;

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  client?.clear();
  client = undefined;
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("部分参考价缺失时只禁用受影响模型的直接写入，完整模型仍可写入", () => {
  render(
    <NewAPIModelPrices
      models={[]}
      managementPrices={catalog.models}
      managementPricesWarning="private-model 参考价不完整，其他完整参考价仍可同步"
      onWriteManagementPrice={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("tab", { name: "远程模型价格" }));
  expect(screen.getByRole("button", { name: "写入平台 private-model" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "写入平台 grok-4.5" })).toBeEnabled();
  expect(screen.getByText("部分参考价不可同步")).toBeVisible();
  expect(screen.queryByText("缓存过期或刷新不完整")).not.toBeInTheDocument();
});

it("批量同步混合参考价时预览缺失原因，确认后只写入完整模型", async () => {
  const write = vi.fn().mockImplementation(async (models) => ({ ...snapshot, models }));
  render(
    <NewAPIModelPrices
      models={snapshot.models}
      onLoadManagementPrices={async () => catalog}
      onWriteModelPrices={write}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
  fireEvent.click(screen.getByRole("button", { name: "批量同步（2）" }));
  const confirm = await screen.findByRole("button", { name: "确认同步 1 个模型" });
  expect(screen.getByText(`跳过：${syncError}`)).toBeVisible();
  fireEvent.click(confirm);
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith([
      { model: "grok-4.5", input_ratio: "1", completion_ratio: "3" },
    ]),
  );
});

it("单项同步缺失参考价时提示具体原因，随后仍可同步其他完整模型", async () => {
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [
      {
        id: "primary",
        name: "主平台",
        base_url: "https://newapi.test",
        user_id: "1",
        admin_key_configured: true,
        updated_at: snapshot.fetched_at,
      },
    ],
    local_groups: [],
    bindings: [],
    sub2api_base_url: "https://sub2api.test",
  });
  vi.spyOn(api, "refreshNewAPIPlatform").mockResolvedValue(snapshot);
  vi.spyOn(api, "managementModelPrices").mockResolvedValue(catalog);
  const write = vi.spyOn(api, "saveNewAPIModelPrices").mockImplementation(async (_id, models) => ({
    ...snapshot,
    models,
  }));
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <NewAPIManagementPage view="prices" />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "同步 private-model 远程模型价格" }));
  expect(await screen.findByText(`private-model：${syncError}`)).toBeVisible();
  expect(write).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "同步 grok-4.5 远程模型价格" }));
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith("primary", [
      { model: "grok-4.5", input_ratio: "1", completion_ratio: "3" },
    ]),
  );
});
