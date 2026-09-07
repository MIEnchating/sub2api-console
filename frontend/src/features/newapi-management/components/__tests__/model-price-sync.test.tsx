import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type NewAPIRemoteSnapshot } from "@/api";
import { NewAPIManagementPage } from "../newapi-management-page";

const platformSnapshot: NewAPIRemoteSnapshot = {
  groups: [],
  models: [{ model: "gpt-5", input_ratio: "1", completion_ratio: "8" }],
  unset_models: [],
  tool_prices: [],
  references: [],
  differences: [],
  fetched_at: "2026-09-06T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());

describe("模型价格远程同步", () => {
  it("点击同步时刷新远程价卡并只写入完全同名模型", async () => {
    vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
      platforms: [
        {
          id: "primary",
          name: "主平台",
          base_url: "https://newapi.example",
          user_id: "1",
          admin_key_configured: true,
          updated_at: "2026-09-06T00:00:00Z",
        },
      ],
      local_groups: [],
      bindings: [],
      sub2api_base_url: "https://sub2api.example",
    });
    vi.spyOn(api, "refreshNewAPIPlatform").mockResolvedValue(platformSnapshot);
    const managementPrices = vi.spyOn(api, "managementModelPrices").mockResolvedValue({
      models: [
        {
          model: "gpt-5-mini",
          input_price: "0.00000025",
          output_price: "0.000002",
          model_ratio: "0.125",
          completion_ratio: "8",
        },
        {
          model: "gpt-5",
          input_price: "0.00000125",
          output_price: "0.00001",
          model_ratio: "0.625",
          completion_ratio: "8",
        },
      ],
    });
    const savePrice = vi
      .spyOn(api, "saveNewAPIModelPrices")
      .mockImplementation(async (_platformId, prices) => ({
        ...platformSnapshot,
        models: prices,
      }));
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <NewAPIManagementPage view="prices" />
      </QueryClientProvider>,
    );

    const syncButton = await screen.findByRole("button", {
      name: "同步 gpt-5 远程模型价格",
    });
    fireEvent.click(syncButton);

    await waitFor(() => expect(managementPrices).toHaveBeenCalledWith("primary"));
    await waitFor(() =>
      expect(savePrice).toHaveBeenCalledWith("primary", [
        {
          model: "gpt-5",
          input_ratio: "0.625",
          completion_ratio: "8",
        },
      ]),
    );
  });
});
