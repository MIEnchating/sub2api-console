import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { PolicyPage } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

function renderPolicy(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["policy"], policy);
  client.setQueryData(["config"], { probes_enabled: true });
  client.setQueryData(["dictionaries", "scheduling_strategy"], { items: [] });
  render(
    <QueryClientProvider client={client}>
      <PolicyPage />
    </QueryClientProvider>,
  );
}

it("保存等待期间继续编辑时保留新草稿，下一次提交使用已确认版本", async () => {
  let finish!: (response: Response) => void;
  const writes: Record<string, unknown>[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (input.endsWith("/api/policy") && init?.method === "PUT") {
      writes.push(JSON.parse(String(init.body)));
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }
    return Promise.resolve(Response.json({ probes_enabled: true }));
  });
  renderPolicy();
  const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
  fireEvent.change(field, { target: { value: "500" } });
  fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  fireEvent.change(field, { target: { value: "600" } });
  await act(async () =>
    finish(
      Response.json({
        ...policy,
        revision: "saved-revision",
        advanced_policy: {
          ...policy.advanced_policy,
          weights: { ...(policy.advanced_policy.weights as object), budget: 500 },
        },
      }),
    ),
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled());
  expect(field).toHaveValue(600);
  fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes[1]).toMatchObject({
    expected_revision: "saved-revision",
    advanced_policy: { weights: { budget: 600 } },
  });
  await act(async () => finish(Response.json(policy)));
});

it("刷新期间输入的新草稿不被迟到响应清除，保留编辑时版本用于冲突检测", async () => {
  let finish!: (response: Response) => void;
  let written: unknown;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (!input.endsWith("/api/policy"))
      return Promise.resolve(Response.json({ probes_enabled: true }));
    if (init?.method === "PUT") {
      written = JSON.parse(String(init.body));
      return Promise.resolve(Response.json({ detail: "策略已更新" }, { status: 409 }));
    }
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  renderPolicy();
  fireEvent.click(screen.getByRole("button", { name: "刷新策略" }));
  await waitFor(() => expect(finish).toBeDefined());
  const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
  fireEvent.change(field, { target: { value: "600" } });
  await act(async () => finish(Response.json({ ...policy, revision: "external-revision" })));
  expect(field).toHaveValue(600);
  fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
  await waitFor(() =>
    expect(written).toMatchObject({
      expected_revision: policy.revision,
      advanced_policy: { weights: { budget: 600 } },
    }),
  );
});

it("策略正在保存时禁用手动刷新，保存失败后保留输入并恢复刷新", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal("fetch", (_input: string, init?: RequestInit) => {
    if (init?.method === "PUT")
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    return Promise.resolve(Response.json({ probes_enabled: true }));
  });
  renderPolicy();
  const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
  fireEvent.change(field, { target: { value: "500" } });
  fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
  await waitFor(() => expect(finish).toBeDefined());
  expect(screen.getByRole("button", { name: "刷新策略" })).toBeDisabled();
  await act(async () => finish(Response.json({ detail: "保存失败" }, { status: 503 })));
  await waitFor(() => expect(screen.getByRole("button", { name: "刷新策略" })).toBeEnabled());
  expect(field).toHaveValue(500);
});

it("保存开始时取消未完成的旧策略读取，迟到结果不覆盖已确认策略", async () => {
  let finishRead!: (response: Response) => void;
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (!input.endsWith("/api/policy"))
      return Promise.resolve(Response.json({ probes_enabled: true }));
    if (init?.method === "PUT")
      return Promise.resolve(
        Response.json({
          ...policy,
          revision: "saved-revision",
          advanced_policy: {
            ...policy.advanced_policy,
            weights: { ...(policy.advanced_policy.weights as object), budget: 500 },
          },
        }),
      );
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finishRead = resolve;
    });
  });
  renderPolicy();
  const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
  fireEvent.change(field, { target: { value: "500" } });
  fireEvent.click(screen.getByRole("button", { name: "刷新策略" }));
  await waitFor(() => expect(finishRead).toBeDefined());
  fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
  await waitFor(() =>
    expect(client.getQueryData<{ revision: string }>(["policy"])?.revision).toBe("saved-revision"),
  );
  expect(signal?.aborted).toBe(true);
  await act(async () => finishRead(Response.json(policy)));
  expect(field).toHaveValue(500);
});

it("手动刷新期间没有新编辑时清除旧草稿并展示最新策略", async () => {
  vi.stubGlobal("fetch", async () => Response.json(policy));
  renderPolicy();
  const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
  fireEvent.change(field, { target: { value: "500" } });
  fireEvent.click(screen.getByRole("button", { name: "刷新策略" }));
  await waitFor(() => expect(field).toHaveValue(1000));
});

it("保存期间启动的后台读取在保存确认后取消，不回退已保存值", async () => {
  let finishSave!: (response: Response) => void;
  let finishRead!: (response: Response) => void;
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (!input.endsWith("/api/policy"))
      return Promise.resolve(Response.json({ probes_enabled: true }));
    if (init?.method === "PUT")
      return new Promise<Response>((resolve) => {
        finishSave = resolve;
      });
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finishRead = resolve;
    });
  });
  renderPolicy();
  const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
  fireEvent.change(field, { target: { value: "500" } });
  fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
  await waitFor(() => expect(finishSave).toBeDefined());
  act(() => {
    void client.refetchQueries({ queryKey: ["policy"], exact: true });
  });
  await waitFor(() => expect(finishRead).toBeDefined());
  await act(async () =>
    finishSave(
      Response.json({
        ...policy,
        revision: "saved-revision",
        advanced_policy: {
          ...policy.advanced_policy,
          weights: { ...(policy.advanced_policy.weights as object), budget: 500 },
        },
      }),
    ),
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled());
  expect(signal?.aborted).toBe(true);
  await act(async () => finishRead(Response.json(policy)));
  expect(field).toHaveValue(500);
});
