import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type ModelPriceCatalog } from "@/api";
import { modelPriceCatalogQueryOptions } from "../model-price-catalog-query";

const catalog: ModelPriceCatalog = { models: [] };
let client: QueryClient;
beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-07T12:00:00Z"));
  client = new QueryClient();
  vi.spyOn(api, "managementModelPrices").mockResolvedValue(catalog);
});
afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("参考价格缓存有效期", () => {
  it("读取后端旧缓存不会延长其 24 小时有效期，到期后才重新请求", async () => {
    const options = modelPriceCatalogQueryOptions("primary");
    client.setQueryData(options.queryKey, { ...catalog, fetched_at: "2026-09-06T13:00:00Z" });
    vi.setSystemTime(new Date("2026-09-07T12:59:59Z"));
    await client.fetchQuery(options);
    expect(api.managementModelPrices).not.toHaveBeenCalled();
    vi.setSystemTime(new Date("2026-09-07T13:00:00Z"));
    await client.fetchQuery(options);
    expect(api.managementModelPrices).toHaveBeenCalledOnce();
  });

  it("部分过期的聚合价卡遵循后端 expires_at 而非最近一次读取时间", async () => {
    const options = modelPriceCatalogQueryOptions("primary");
    client.setQueryData(options.queryKey, {
      ...catalog,
      fetched_at: "2026-09-07T11:00:00Z",
      expires_at: "2026-09-07T11:59:59Z",
    });
    await client.fetchQuery(options);
    expect(api.managementModelPrices).toHaveBeenCalledOnce();
  });

  it("缓存标为刷新不完整时重新请求，不将其视为有效价卡", async () => {
    const options = modelPriceCatalogQueryOptions("primary");
    client.setQueryData(options.queryKey, {
      ...catalog,
      stale: true,
      expires_at: "2026-09-08T12:00:00Z",
    });
    await client.fetchQuery(options);
    expect(api.managementModelPrices).toHaveBeenCalledOnce();
  });

  it("无缓存的并发使用合并为一次请求，不同平台分别缓存", async () => {
    await Promise.all([
      client.fetchQuery(modelPriceCatalogQueryOptions("primary")),
      client.fetchQuery(modelPriceCatalogQueryOptions("primary")),
    ]);
    expect(api.managementModelPrices).toHaveBeenCalledOnce();
    await client.fetchQuery(modelPriceCatalogQueryOptions("secondary"));
    expect(api.managementModelPrices).toHaveBeenCalledTimes(2);
    expect(api.managementModelPrices).toHaveBeenLastCalledWith("secondary");
  });
});
