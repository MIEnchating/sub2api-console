import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type OnboardingContext } from "@/api";
import { upstream } from "../../__tests__/onboarding-fixture";
import { useOnboardingPreparation } from "../use-onboarding-preparation";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});

function renderPreparation() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderHook(useOnboardingPreparation, {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
}

it("切换上游取消旧请求，旧结果迟到不会覆盖当前上游", async () => {
  let finishFirst!: (value: OnboardingContext) => void;
  let firstSignal: AbortSignal | undefined;
  const second = { upstream: { ...upstream, host: "second.example.test" }, candidates: [] };
  vi.spyOn(api, "prepareOnboarding").mockImplementation((host, signal) => {
    if (host === upstream.host) {
      firstSignal = signal;
      return new Promise((resolve) => {
        finishFirst = resolve;
      });
    }
    return Promise.resolve(second);
  });
  const hook = renderPreparation();
  act(() => hook.result.current.load(upstream.host));
  await waitFor(() => expect(hook.result.current.isPending).toBe(true));
  act(() => hook.result.current.load(second.upstream.host));
  await waitFor(() => expect(hook.result.current.data).toEqual(second));
  expect(firstSignal?.aborted).toBe(true);
  await act(async () => finishFirst({ upstream, candidates: [] }));
  expect(hook.result.current.data).toEqual(second);
  expect(hook.result.current.isPending).toBe(false);
});

it("刷新同一上游时保留候选，读取失败后可直接重试恢复", async () => {
  let failRefresh!: (error: Error) => void;
  const prepared = { upstream, candidates: [] };
  vi.spyOn(api, "prepareOnboarding")
    .mockResolvedValueOnce(prepared)
    .mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          failRefresh = reject;
        }),
    )
    .mockResolvedValueOnce(prepared);
  const hook = renderPreparation();
  act(() => hook.result.current.load(upstream.host));
  await waitFor(() => expect(hook.result.current.data).toEqual(prepared));
  act(() => hook.result.current.load(upstream.host));
  await waitFor(() => expect(hook.result.current.isPending).toBe(true));
  expect(hook.result.current.data).toEqual(prepared);
  await act(async () => failRefresh(new Error("上游暂时不可用")));
  await waitFor(() => expect(hook.result.current.error?.message).toBe("上游暂时不可用"));
  expect(hook.result.current.data).toEqual(prepared);
  expect(hook.result.current.isPending).toBe(false);
  act(() => hook.result.current.load(upstream.host));
  await waitFor(() => expect(hook.result.current.data).toEqual(prepared));
  expect(hook.result.current.error).toBeNull();
});
