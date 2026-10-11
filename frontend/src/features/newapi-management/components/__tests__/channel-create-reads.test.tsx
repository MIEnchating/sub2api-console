import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { seedDictionaries } from "@/test/dictionary-fixtures";
import { ChannelCreateDialog } from "../channel-create-dialog";

let client: QueryClient;
const groups = [{ id: "default", name: "默认", ratio: "1" }];

function configurationResponse(url: RequestInfo | URL): Response {
  const path = String(url);
  if (path.endsWith("/groups")) return Response.json({ groups });
  if (path.endsWith("/refresh")) return Response.json({ groups, models: [] });
  if (path === "/api/auth-recovery/config") {
    return Response.json({
      vault_entries: [
        {
          entry: "运营账号",
          hosts: ["sub.example.test"],
          has_username: true,
          has_password: true,
          username_is_email: true,
          header_names: [],
        },
      ],
    });
  }
  if (path.endsWith("/channel-key")) {
    return Response.json({
      key_id: "key-6",
      name: "标准",
      group_id: "6",
      endpoints: [{ name: "上游接口", base_url: "https://api.example.test", default: true }],
    });
  }
  throw new Error(`Unexpected request: ${path}`);
}

function ChannelCreation(): React.ReactElement | null {
  const [open, setOpen] = useState(true);
  return open ? (
    <ChannelCreateDialog
      platformId="primary"
      groups={[{ id: "6", name: "标准", ratio: "1" }]}
      baseURL="https://sub.example.test"
      onClose={() => setOpen(false)}
    />
  ) : null;
}

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  seedDictionaries(client, { group: [{ value: "6", name: "标准" }] });
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
});

function mount(): ReturnType<typeof render> {
  return render(
    <QueryClientProvider client={client}>
      <ChannelCreation />
    </QueryClientProvider>,
  );
}

it("新增渠道只读取分组目录即可配置，不请求包含价格的完整快照", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (url) => configurationResponse(url));
  mount();
  expect(await screen.findByRole("button", { name: "创建密钥" })).toBeVisible();
  expect(fetch.mock.calls.map(([url]) => String(url))).toEqual(
    expect.arrayContaining(["/api/newapi/platforms/primary/groups", "/api/auth-recovery/config"]),
  );
  expect(fetch.mock.calls.some(([url]) => String(url).endsWith("/refresh"))).toBe(false);
});

it("关闭配置读取中的新增渠道弹窗时中止分组请求", async () => {
  let signal: AbortSignal | null | undefined;
  let complete!: (response: Response) => void;
  vi.spyOn(globalThis, "fetch").mockImplementation((url, options) => {
    if (String(url).endsWith("/groups") || String(url).endsWith("/refresh")) {
      signal = options?.signal;
      return new Promise((resolve) => {
        complete = resolve;
      });
    }
    return Promise.resolve(configurationResponse(url));
  });
  const user = userEvent.setup();
  mount();
  await screen.findByRole("status", { name: "正在读取渠道创建配置" });
  await user.click(screen.getByRole("button", { name: "取消" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await act(async () => {
    complete(Response.json({ groups }));
  });
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData(["newapi-remote-groups", "primary"])).toBeUndefined();
});

it.each(["成功", "失败"])("取消模型读取后忽略迟到的%s响应并允许重新读取", async (outcome) => {
  let signal: AbortSignal | null | undefined;
  let complete!: (response: Response) => void;
  let fail!: (error: Error) => void;
  let modelReads = 0;
  vi.spyOn(globalThis, "fetch").mockImplementation((url, options) => {
    if (String(url).endsWith("/channel-models")) {
      modelReads += 1;
      if (modelReads > 1) return Promise.resolve(Response.json({ models: ["fresh-model"] }));
      signal = options?.signal;
      return new Promise((resolve, reject) => {
        complete = resolve;
        fail = reject;
      });
    }
    return Promise.resolve(configurationResponse(url));
  });
  const notify = vi.spyOn(toast, "error");
  const user = userEvent.setup();
  mount();
  await user.click(await screen.findByRole("combobox", { name: "Sub2API 分组" }));
  await user.click(await screen.findByRole("option", { name: "标准" }));
  await user.click(screen.getByRole("button", { name: "创建密钥" }));
  await user.click(await screen.findByRole("button", { name: "从上游获取" }));
  const modelDialog = await screen.findByRole("dialog", { name: "选择上游模型" });
  await user.click(within(modelDialog).getByRole("button", { name: "取消" }));
  await act(async () => {
    if (outcome === "成功") complete(Response.json({ models: ["stale-model"] }));
    else fail(new Error("迟到的读取失败"));
  });
  expect(signal?.aborted).toBe(true);
  expect(notify).not.toHaveBeenCalled();
  await waitFor(() => expect(modelDialog).not.toBeInTheDocument());
  await user.click(await screen.findByRole("button", { name: "从上游获取" }));
  expect(await screen.findByRole("checkbox", { name: /fresh-model/ })).toBeChecked();
  expect(screen.queryByRole("checkbox", { name: /stale-model/ })).not.toBeInTheDocument();
});
