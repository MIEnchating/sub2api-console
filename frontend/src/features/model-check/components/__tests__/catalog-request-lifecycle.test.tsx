import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useAccountCollection } from "@/features/accounts/hooks/use-account-collection";
import { RegularCheckPanel } from "../regular-check-panel";

const queries = [
  { path: "/api/accounts", key: ["accounts"], value: [] },
  { path: "/api/model-checks/account-statuses", key: ["model-check-account-statuses"], value: [] },
];
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function SharedAccounts() {
  useAccountCollection(false);
  return null;
}

function setup(shared: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  const pending = new Map<
    string,
    { signal: AbortSignal | null | undefined; release: (response: Response) => void }
  >();
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    const path = String(input);
    if (!queries.some((query) => query.path === path)) throw new Error(`Unexpected read: ${path}`);
    return new Promise<Response>((release) => pending.set(path, { signal: init?.signal, release }));
  });
  vi.stubGlobal("fetch", fetcher);
  const tree = (showPanel: boolean) => (
    <QueryClientProvider client={client}>
      {showPanel && <RegularCheckPanel />}
      {shared && <SharedAccounts />}
    </QueryClientProvider>
  );
  const view = render(tree(true));
  return { ...view, client, pending, fetcher, closePanel: () => view.rerender(tree(false)) };
}

it("离开模型检测时取消账号和状态读取，迟到结果不写回缓存", async () => {
  const view = setup(false);
  await waitFor(() => expect(view.pending.size).toBe(2));
  try {
    view.unmount();
    for (const query of queries) expect(view.pending.get(query.path)?.signal?.aborted).toBe(true);
  } finally {
    await act(async () => {
      for (const query of queries)
        view.pending.get(query.path)!.release(Response.json(query.value));
    });
  }
  for (const query of queries) expect(view.client.getQueryData(query.key)).toBeUndefined();
  expect(view.fetcher.mock.calls.every(([, init]) => !init?.method)).toBe(true);
});

it("模型检测离开后共享账号观察者保留目录和状态读取，最后卸载才取消", async () => {
  const view = setup(true);
  await waitFor(() => expect(view.pending.size).toBe(2));
  try {
    view.closePanel();
    expect(view.pending.get("/api/accounts")?.signal?.aborted).toBe(false);
    expect(view.pending.get("/api/model-checks/account-statuses")?.signal?.aborted).toBe(false);
    view.unmount();
    for (const query of queries) expect(view.pending.get(query.path)?.signal?.aborted).toBe(true);
  } finally {
    await act(async () => {
      for (const query of queries)
        view.pending.get(query.path)!.release(Response.json(query.value));
    });
  }
  expect(view.fetcher).toHaveBeenCalledTimes(2);
});
