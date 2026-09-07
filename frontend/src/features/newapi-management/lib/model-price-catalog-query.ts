import { queryOptions } from "@tanstack/react-query";

import { api, type ModelPriceCatalog } from "@/api";

const cacheLifetime = 24 * 60 * 60 * 1000;

function catalogStaleTime(catalog: ModelPriceCatalog | undefined, updatedAt: number): number {
  if (!catalog || catalog.stale) return 0;
  let expiresAt = updatedAt + cacheLifetime;
  if (catalog.fetched_at) {
    const fetchedAt = Date.parse(catalog.fetched_at);
    if (!Number.isFinite(fetchedAt) || fetchedAt > updatedAt) return 0;
    expiresAt = Math.min(expiresAt, fetchedAt + cacheLifetime);
  }
  if (catalog.expires_at) {
    const serverExpiry = Date.parse(catalog.expires_at);
    if (!Number.isFinite(serverExpiry)) return 0;
    expiresAt = Math.min(expiresAt, serverExpiry);
  }
  return Math.max(0, expiresAt - updatedAt);
}

export function modelPriceCatalogQueryOptions(platformId: string) {
  return queryOptions({
    queryKey: ["newapi-management-model-prices", platformId],
    queryFn: () => api.managementModelPrices(platformId),
    retry: false,
    gcTime: cacheLifetime,
    // Use the backend expiry; reading its cache must not restart the 24-hour lifetime.
    staleTime: (query) => catalogStaleTime(query.state.data, query.state.dataUpdatedAt),
  });
}
