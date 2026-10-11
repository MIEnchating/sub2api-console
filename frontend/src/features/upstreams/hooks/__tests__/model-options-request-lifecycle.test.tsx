import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { useOnboardingModelOptions } from "../use-onboarding-model-options";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("模型选项离页时取消读取，再次进入继续同一任务且不重复创建", async () => {
  const task: Task = {
    id: "models-1",
    operation: "onboarding-model-options",
    skill: "onboarding",
    status: "queued",
    progress: 0,
    message: "读取模型",
    result: {},
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
  const completed = { ...task, status: "succeeded", result: { models: ["model-current"] } };
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  let reads = 0;
  let starts = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        starts++;
        return Promise.resolve(Response.json(task));
      }
      reads++;
      if (reads > 1) return Promise.resolve(Response.json(completed));
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const wrapper = (props: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  );
  const first = renderHook(() => useOnboardingModelOptions("models.example.test", "group-a"), {
    wrapper,
  });
  act(() => first.result.current.load());
  await waitFor(() => expect(reads).toBe(1));
  first.unmount();
  await act(async () => finish(Response.json(completed)));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData(["onboarding-model-options-task", task.id])).toBeUndefined();
  const next = renderHook(() => useOnboardingModelOptions("models.example.test", "group-a"), {
    wrapper,
  });
  await waitFor(() => expect(next.result.current.models).toEqual(["model-current"]));
  expect(starts).toBe(1);
  expect(reads).toBe(2);
});
