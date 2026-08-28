import { QueryCache, QueryClient } from "@tanstack/react-query";

import { notifyOperationError } from "./operation-feedback";

export function createConsoleQueryClient(): QueryClient {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => notifyOperationError(error, "请求失败"),
    }),
    defaultOptions: {
      queries: { staleTime: 15_000, refetchOnWindowFocus: false },
    },
  });
}
