import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { NewAPIManagementPage } from "../newapi-management-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("批量价格预览读取中离页取消请求，不把取消提示为同步失败", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const notify = vi.spyOn(toast, "error");
  const signal = holdEndpoint("/management-model-prices");
  const view = mount();
  fireEvent.click(await screen.findByRole("checkbox", { name: "选择模型 test-model" }));
  fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
  await screen.findByRole("status", { name: "正在准备批量价格预览" });
  await act(async () => view.unmount());
  expect(signal()?.aborted).toBe(true);
  expect(notify).not.toHaveBeenCalled();
});
const snapshot = {
  groups: [],
  models: [{ model: "test-model", input_ratio: "1", completion_ratio: "2" }],
  unset_models: [],
  references: [],
  differences: [],
  tool_prices: [],
};
const workspace = {
  platforms: [
    {
      id: "primary",
      name: "测试",
      base_url: "https://newapi.example.test",
      user_id: "1",
      admin_key_configured: true,
      updated_at: "",
    },
  ],
  local_groups: [],
  bindings: [],
};
function mount() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NewAPIManagementPage view="prices" />
    </QueryClientProvider>,
  );
}
function holdEndpoint(endpoint: string) {
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith(endpoint)) {
      signal = init?.signal;
      return new Promise<Response>(() => {});
    }
    if (url.endsWith("/refresh")) return Promise.resolve(Response.json(snapshot));
    if (url.includes("/dictionaries")) return Promise.resolve(Response.json({ items: [] }));
    return Promise.resolve(Response.json(workspace));
  });
  return () => signal;
}

it("离开价格页面会取消未完成的共享快照读取", async () => {
  const signal = holdEndpoint("/refresh");
  const view = mount();
  await screen.findByRole("status", { name: "正在加载模型价格" });
  view.unmount();
  expect(signal()?.aborted).toBe(true);
});

it("离开价格页面会取消参考价卡读取", async () => {
  const signal = holdEndpoint("/management-model-prices");
  const view = mount();
  fireEvent.click(await screen.findByRole("tab", { name: "远程模型价格" }));
  await waitFor(() =>
    expect(client.isFetching({ queryKey: ["newapi-management-model-prices", "primary"] })).toBe(1),
  );
  view.unmount();
  expect(signal()?.aborted).toBe(true);
});

it("原始价卡读取期间关闭弹窗会取消请求并移除原文缓存", async () => {
  const notify = vi.spyOn(toast, "error");
  const signal = holdEndpoint("/remote-model-prices/raw");
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "查看原始价卡" }));
  const dialog = await screen.findByRole("dialog", { name: "远程价卡原始文件" });
  await screen.findByRole("status", { name: "正在读取远程价卡原始文件" });
  fireEvent.click(within(dialog).getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(signal()?.aborted).toBe(true));
  await waitFor(() =>
    expect(
      client.getQueryCache().find({ queryKey: ["newapi-remote-model-pricing-source", "primary"] }),
    ).toBeUndefined(),
  );
  expect(notify).not.toHaveBeenCalled();
});

it("价格快照短暂离页可复用，闲置五分钟后释放内存缓存", async () => {
  holdEndpoint("unused");
  const view = mount();
  await screen.findByRole("button", { name: "查看原始价卡" });
  vi.useFakeTimers();
  view.unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4 * 60_000);
  });
  expect(client.getQueryData(["newapi-remote-snapshot", "primary"])).toEqual(snapshot);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(60_000);
  });
  expect(client.getQueryData(["newapi-remote-snapshot", "primary"])).toBeUndefined();
});
