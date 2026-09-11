import { Toaster, toast } from "sonner";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type ModelPriceCatalog, type NewAPIRemoteSnapshot } from "@/api";
import { NewAPIManagementPage } from "../newapi-management-page";

const snapshot: NewAPIRemoteSnapshot = {
  groups: [],
  models: [{ model: "model-a", input_ratio: "0.5", completion_ratio: "4" }],
  unset_models: [],
  references: [],
  tool_prices: [],
  differences: [],
  fetched_at: "2026-09-07T00:00:00Z",
};
const catalog: ModelPriceCatalog = {
  models: [
    {
      model: "model-a",
      input_price: "0.000001",
      output_price: "0.000004",
      model_ratio: "0.5",
      completion_ratio: "4",
    },
  ],
};
let client: QueryClient;

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [
      {
        id: "primary",
        name: "测试平台",
        base_url: "https://newapi.example",
        user_id: "1",
        admin_key_configured: true,
        updated_at: snapshot.fetched_at,
      },
    ],
    local_groups: [],
    bindings: [],
    sub2api_base_url: "https://sub2api.example",
  });
  vi.spyOn(api, "refreshNewAPIPlatform").mockResolvedValue(snapshot);
  vi.spyOn(api, "managementModelPrices").mockResolvedValue(catalog);
});
afterEach(() => {
  toast.dismiss();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("模型价格操作复用参考缓存", () => {
  it("有效缓存用于比较、批量预览和单项同步，不重复请求参考价", async () => {
    client.setQueryData(["newapi-management-model-prices", "primary"], {
      ...catalog,
      expires_at: new Date(Date.now() + 23 * 60 * 60 * 1000).toISOString(),
    });
    const write = vi.spyOn(api, "saveNewAPIModelPrices").mockResolvedValue(snapshot);
    render(
      <QueryClientProvider client={client}>
        <Toaster />
        <NewAPIManagementPage view="prices" />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "比较模型价格" }));
    await screen.findByText("一致", { exact: true });
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    expect(await screen.findByRole("button", { name: "确认同步 1 个模型" })).toBeEnabled();
    expect(write).not.toHaveBeenCalled();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "取消" }));
    fireEvent.click(screen.getByRole("button", { name: "同步 model-a 远程模型价格" }));
    await waitFor(() => expect(write).toHaveBeenCalledWith("primary", snapshot.models));
    expect(api.managementModelPrices).not.toHaveBeenCalled();
  });

  it("缓存过期时重新拉取但保留后台比较状态，失败后不允许批量确认", async () => {
    client.setQueryData(["newapi-management-model-prices", "primary"], catalog);
    render(
      <QueryClientProvider client={client}>
        <Toaster />
        <NewAPIManagementPage view="prices" />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "比较模型价格" }));
    await screen.findByText("一致", { exact: true });
    let rejectPrices: (error: Error) => void = () => {};
    vi.mocked(api.managementModelPrices).mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectPrices = reject;
        }),
    );
    act(() =>
      client.setQueryData(["newapi-management-model-prices", "primary"], {
        ...catalog,
        expires_at: new Date(Date.now() - 1000).toISOString(),
      }),
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    await screen.findByRole("status", { name: "正在准备批量价格预览" });
    expect(screen.getByText("一致", { exact: true })).toBeInTheDocument();
    expect(screen.queryByText("比较中", { exact: true })).not.toBeInTheDocument();
    await act(async () => rejectPrices(new Error("参考接口暂时不可用")));
    expect(await screen.findByText("参考接口暂时不可用")).toBeVisible();
    expect(within(screen.getByRole("dialog")).queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "确认同步 0 个模型" })).toBeDisabled();
    expect(screen.getByText("一致", { exact: true })).toBeInTheDocument();
  });

  it("主动强制刷新仍发请求，刷新成功后的批量预览使用新缓存", async () => {
    client.setQueryData(["newapi-management-model-prices", "primary"], catalog);
    const refresh = vi.spyOn(api, "refreshManagementModelPrices").mockResolvedValue({
      ...catalog,
      expires_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
    });
    render(
      <QueryClientProvider client={client}>
        <Toaster />
        <NewAPIManagementPage view="prices" />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "强制刷新参考价格" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "强制刷新参考价格" })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页模型" }));
    fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
    expect(await screen.findByRole("button", { name: "确认同步 1 个模型" })).toBeEnabled();
    expect(api.managementModelPrices).not.toHaveBeenCalled();
  });
});
