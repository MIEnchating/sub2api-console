import { QueryClient, QueryClientProvider, onlineManager } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useModelSyncBlocking } from "../use-model-sync-blocking";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

function setup(options: { hold?: "save" | "preview"; fail?: "save" | "preview" } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: 2, retryDelay: 0 } },
  });
  clients.push(client);
  const requests: string[] = [];
  const bodies: unknown[] = [];
  let failure = options.fail;
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let previewSignal: AbortSignal | null | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      requests.push(`${init?.method ?? "GET"} ${path}`);
      const phase = path.endsWith("/preview") ? "preview" : "save";
      if (phase === "preview") previewSignal = init?.signal;
      if (init?.body) bodies.push(JSON.parse(String(init.body)));
      if (phase === options.hold) await gate;
      if (phase === failure)
        return Response.json({ code: "test_failure", detail: "请求失败，请重试" }, { status: 503 });
      if (phase === "preview")
        return Response.json({
          account_count: 1,
          accounts_with_catalog: 1,
          accounts: [],
          models: [],
          blocked_patterns: ["model-a"],
          blocked_models: ["model-a"],
          fingerprint: "fresh",
        });
      return Response.json({ blocked_patterns: ["model-a"] });
    }),
  );
  const updated = vi.fn();
  const hook = renderHook(
    () => useModelSyncBlocking({ accountIds: ["41"], onPreviewUpdated: updated }),
    {
      wrapper: (props: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
      ),
    },
  );
  return {
    ...hook,
    client,
    requests,
    bodies,
    updated,
    release,
    signal: () => previewSignal,
    recover: () => {
      failure = undefined;
    },
  };
}

it("屏蔽单个模型只提交追加请求，不先读后覆盖全局规则", async () => {
  const state = setup();
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.updated).toHaveBeenCalled());
  expect(state.requests).toEqual([
    "POST /api/config/model-sync/blocked-models",
    "POST /api/management/accounts/models/preview",
  ]);
  expect(state.bodies[0]).toEqual({ model: "model-a" });
});

it("关闭窗口后保存返回仍失效缓存，但不启动旧窗口预览", async () => {
  const state = setup({ hold: "save" });
  state.client.setQueryData(["account-model-sync-preview", ["41"]], { fingerprint: "old" });
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.requests).toHaveLength(1));
  state.unmount();
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.updated).not.toHaveBeenCalled();
  expect(state.requests.some((path) => path.endsWith("/preview"))).toBe(false);
  expect(state.client.getQueryState(["account-model-sync-preview", ["41"]])?.isInvalidated).toBe(
    true,
  );
});

it("预览中关闭窗口会取消读取，迟到预览不覆盖缓存或调用旧回调", async () => {
  const state = setup({ hold: "preview" });
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.requests.some((path) => path.endsWith("/preview"))).toBe(true));
  state.unmount();
  expect(state.signal()?.aborted).toBe(true);
  state.client.setQueryData(["account-model-sync-preview", ["41"]], { fingerprint: "new-window" });
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.updated).not.toHaveBeenCalled();
  expect(state.client.getQueryData(["account-model-sync-preview", ["41"]])).toEqual({
    fingerprint: "new-window",
  });
});

it("同轮重复屏蔽只发送一次写入", async () => {
  const state = setup({ hold: "save" });
  act(() => {
    state.result.current.block("model-a");
    state.result.current.block("model-a");
  });
  await waitFor(() => expect(state.requests.length).toBeGreaterThan(0));
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.requests.filter((path) => !path.endsWith("/preview"))).toHaveLength(1);
});

it("保存失败不自动重试，手动重试后可恢复", async () => {
  const state = setup({ fail: "save" });
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.requests.length).toBeGreaterThan(0));
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.requests).toHaveLength(1);
  state.recover();
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.updated).toHaveBeenCalled());
});

it("预览失败保留已保存规则，仅重试预览即可恢复", async () => {
  const state = setup({ fail: "preview" });
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.result.current.refreshFailed).toBe(true));
  expect(state.result.current.savedSettings?.blocked_patterns).toEqual(["model-a"]);
  expect(state.requests.filter((path) => path.endsWith("/preview"))).toHaveLength(1);
  state.recover();
  act(() => state.result.current.refresh());
  await waitFor(() => expect(state.updated).toHaveBeenCalled());
  expect(state.result.current.savedSettings).toBeNull();
});

it("离线标记不会让屏蔽写入延迟到恢复联网后发送", async () => {
  const state = setup();
  onlineManager.setOnline(false);
  act(() => state.result.current.block("model-a"));
  await waitFor(() => expect(state.updated).toHaveBeenCalled());
});
