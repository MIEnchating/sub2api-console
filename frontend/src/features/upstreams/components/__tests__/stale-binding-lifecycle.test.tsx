import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { StaleBindingCleanup } from "../stale-binding-cleanup";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup() {
  const client = new QueryClient();
  clients.push(client);
  const onChanged = vi.fn();
  const initial = {
    host: "api.example.test",
    bindingId: 7,
    upstreamId: "up-example",
    accountId: "142",
    upstreamKeyId: "key-142",
    groupId: "6",
    groupName: "测试分组",
    disabled: false,
    onChanged,
  };
  function view(patch: Partial<typeof initial> = {}) {
    return (
      <QueryClientProvider client={client}>
        <StaleBindingCleanup {...initial} {...patch} />
      </QueryClientProvider>
    );
  }
  return { ...render(view()), view, onChanged, client };
}

it.each([
  { host: "other.example.test" },
  { bindingId: 8 },
  { upstreamId: "up-other" },
  { accountId: "143" },
  { upstreamKeyId: "key-new" },
  { groupId: "8" },
])("确认期间身份变为 %j 时关闭旧确认且不执行清理", async (patch) => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  const page = setup();
  fireEvent.click(screen.getByRole("button", { name: "清理失效绑定" }));
  expect(screen.getByRole("button", { name: "确认清理" })).toBeEnabled();
  page.rerender(page.view(patch));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(fetch).not.toHaveBeenCalled();
});

it("旧绑定清理成功时刷新缓存但不关闭新确认或调用新回调", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn().mockReturnValue(
      new Promise<Response>((done) => {
        resolve = done;
      }),
    ),
  );
  const success = vi.spyOn(toast, "success");
  const page = setup();
  page.client.setQueryData(["upstreams"], { hosts: [] });
  fireEvent.click(screen.getByRole("button", { name: "清理失效绑定" }));
  fireEvent.click(screen.getByRole("button", { name: "确认清理" }));
  await screen.findByRole("button", { name: "正在清理…" });
  const nextChanged = vi.fn();
  page.rerender(page.view({ bindingId: 8, onChanged: nextChanged }));
  fireEvent.click(screen.getByRole("button", { name: "清理失效绑定" }));
  await act(async () => resolve(new Response(JSON.stringify({ binding_id: 7 }), { status: 200 })));
  expect(screen.getByRole("dialog")).toHaveTextContent("绑定 ID 8");
  expect(page.client.getQueryState(["upstreams"])?.isInvalidated).toBe(true);
  expect(page.onChanged).not.toHaveBeenCalled();
  expect(nextChanged).not.toHaveBeenCalled();
  expect(success).not.toHaveBeenCalled();
});

it("清理期间卸载后收到失败不展示过期错误提示", async () => {
  let reject!: (error: Error) => void;
  const fetch = vi.fn().mockReturnValue(
    new Promise<Response>((_, fail) => {
      reject = fail;
    }),
  );
  vi.stubGlobal("fetch", fetch);
  const notify = vi.spyOn(toast, "error");
  const page = setup();
  fireEvent.click(screen.getByRole("button", { name: "清理失效绑定" }));
  fireEvent.click(screen.getByRole("button", { name: "确认清理" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  page.unmount();
  await act(async () => reject(new Error("连接断开")));
  expect(notify).not.toHaveBeenCalled();
});
