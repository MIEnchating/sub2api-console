import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { seedDictionaries } from "@/test/dictionary-fixtures";
import { AlertPolicyPage } from "../alert-policy-page";
import { groups, notificationStatus, policy } from "./fixtures";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("离开告警策略页会取消策略与通知配置读取，重开重新请求并忽略旧响应", async () => {
  const requests: {
    url: string;
    signal: AbortSignal | null | undefined;
    resolve: (value: Response) => void;
  }[] = [];
  vi.stubGlobal(
    "fetch",
    (input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((resolve) =>
        requests.push({ url: String(input), signal: init?.signal, resolve }),
      ),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  seedDictionaries(client, { group: [] });
  client.setQueryData(["groups", "options"], groups);
  const page = (
    <QueryClientProvider client={client}>
      <AlertPolicyPage onOpenSettings={() => undefined} />
    </QueryClientProvider>
  );
  const first = render(page);
  await waitFor(() => expect(requests).toHaveLength(2));
  first.unmount();
  for (const request of requests) expect(request.signal?.aborted).toBe(true);
  render(page);
  await waitFor(() => expect(requests).toHaveLength(4));
  await act(async () => {
    for (const request of requests.slice(0, 2)) {
      request.resolve(Response.json(request.url.endsWith("policy") ? policy : notificationStatus));
    }
  });
  expect(client.getQueryData(["alert-policy"])).toBeUndefined();
  expect(client.getQueryData(["notification-status"])).toBeUndefined();
  await act(async () => {
    for (const request of requests.slice(2)) {
      request.resolve(Response.json(request.url.endsWith("policy") ? policy : notificationStatus));
    }
  });
  await waitFor(() => expect(client.getQueryData(["alert-policy"])).toEqual(policy));
  expect(client.getQueryData(["notification-status"])).toEqual(notificationStatus);
});
