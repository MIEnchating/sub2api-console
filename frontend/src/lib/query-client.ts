import { QueryCache, QueryClient } from "@tanstack/react-query";

import { ApiError } from "@/api";
import { notifyOperationError } from "./operation-feedback";
import { isSessionExpiredError } from "./session-auth";

function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  if (isSessionExpiredError(error)) return false;
  if (
    error instanceof ApiError &&
    error.status >= 400 &&
    error.status < 500 &&
    error.status !== 408 &&
    error.status !== 429
  )
    return false;
  return failureCount < 3;
}

export function createConsoleQueryClient(): QueryClient {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => notifyOperationError(error, "请求失败"),
    }),
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        refetchOnWindowFocus: false,
        retry: shouldRetryQuery,
      },
    },
  });
}
