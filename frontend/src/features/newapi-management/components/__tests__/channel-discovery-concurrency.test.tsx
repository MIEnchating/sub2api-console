import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { NewAPIChannel } from "@/api";
import { ChannelModelDiscovery } from "../channel-model-discovery";

const channels: NewAPIChannel[] = Array.from({ length: 5 }, (_, index) => ({
  id: String(index + 1),
  name: `渠道 ${index + 1}`,
  type: 59,
  status: 1,
  groups: [],
  models: [],
  version: "a".repeat(64),
}));
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ChannelModelDiscovery
        platformId="primary"
        channels={channels}
        onClose={() => {}}
        onConfirm={() => {}}
      />
    </QueryClientProvider>,
  );
}

function holdReads() {
  const pending: { signal?: AbortSignal | null; resolve: (value: Response) => void }[] = [];
  vi.stubGlobal(
    "fetch",
    (_input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((resolve) => pending.push({ signal: init?.signal, resolve })),
  );
  return pending;
}

it("批量获取最多同时读取四个渠道，全部完成后才显示共同模型并保留首渠道顺序", async () => {
  const pending = holdReads();
  mount();
  await waitFor(() => expect(pending).toHaveLength(4));
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
  await act(async () => {
    pending[3].resolve(Response.json({ models: ["a", "z", "fourth-only"] }));
    pending[2].resolve(Response.json({ models: ["z", "a"] }));
    pending[1].resolve(Response.json({ models: ["a", "z"] }));
    pending[0].resolve(Response.json({ models: ["z", "a", "first-only"] }));
  });
  await waitFor(() => expect(pending).toHaveLength(5));
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  await act(async () => pending[4].resolve(Response.json({ models: ["a", "z"] })));
  await waitFor(() => expect(screen.getAllByRole("checkbox")).toHaveLength(2));
  expect(screen.getAllByRole("checkbox").map((item) => item.getAttribute("aria-label"))).toEqual([
    "选择模型 z",
    "选择模型 a",
  ]);
});

it("批量任一读取失败时取消同批请求且不启动剩余渠道，重试重新读取完整范围", async () => {
  const pending = holdReads();
  mount();
  await waitFor(() => expect(pending).toHaveLength(4));
  await act(async () =>
    pending[1].resolve(Response.json({ detail: "渠道版本已变化" }, { status: 409 })),
  );
  await screen.findByRole("button", { name: "重新读取" });
  expect(pending).toHaveLength(4);
  for (const item of pending) expect(item.signal?.aborted).toBe(true);
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  await waitFor(() => expect(pending).toHaveLength(8));
  await act(async () => {
    for (const item of pending.slice(4)) item.resolve(Response.json({ models: ["fresh"] }));
  });
  await waitFor(() => expect(pending).toHaveLength(9));
  await act(async () => pending[8].resolve(Response.json({ models: ["fresh"] })));
  expect(await screen.findByRole("checkbox", { name: /fresh/ })).toBeVisible();
});

it("关闭批量模型读取后中止在途请求，晚到响应不再触发下批读取", async () => {
  const pending = holdReads();
  const view = mount();
  await waitFor(() => expect(pending).toHaveLength(4));
  view.unmount();
  for (const item of pending) expect(item.signal?.aborted).toBe(true);
  await act(async () => {
    for (const item of pending) item.resolve(Response.json({ models: ["late"] }));
  });
  expect(pending).toHaveLength(4);
});

it("首批没有共同模型时仍核对剩余渠道，后续版本失败不能冒充空结果", async () => {
  const pending = holdReads();
  mount();
  await waitFor(() => expect(pending).toHaveLength(4));
  await act(async () => {
    pending.forEach((item, index) => item.resolve(Response.json({ models: [`only-${index}`] })));
  });
  await waitFor(() => expect(pending).toHaveLength(5));
  await act(async () =>
    pending[4].resolve(Response.json({ detail: "渠道版本已变化" }, { status: 409 })),
  );
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(screen.queryByText("没有可新增的上游模型")).not.toBeInTheDocument();
});
