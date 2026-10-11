import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { NewAPIChannel } from "@/api";
import { ExistingChannels } from "../existing-channels";
import { ChannelGroupMemberDialog } from "../channel-group-member-dialog";
import { renderWithRouter } from "./render-with-router";

let client: QueryClient;
beforeEach(() => vi.stubGlobal("scrollTo", vi.fn()));
const channel: NewAPIChannel = {
  id: "41",
  name: "生产渠道",
  type: 59,
  status: 1,
  models: ["model"],
  groups: [],
  version: "a".repeat(64),
};

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function createClient(): QueryClient {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return client;
}

it("离开渠道列表取消列表与分组读取，晚到结果不进入缓存", async () => {
  const pending: {
    url: string;
    signal?: AbortSignal | null;
    resolve: (value: Response) => void;
  }[] = [];
  vi.stubGlobal(
    "fetch",
    (input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((resolve) =>
        pending.push({ url: String(input), signal: init?.signal, resolve }),
      ),
  );
  renderWithRouter(
    <QueryClientProvider client={createClient()}>
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(pending).toHaveLength(2));
  cleanup();
  for (const request of pending) expect(request.signal?.aborted).toBe(true);
  await act(async () => {
    for (const request of pending)
      request.resolve(
        Response.json(
          request.url.endsWith("channel-groups")
            ? { groups: [], version: "" }
            : { items: [channel], total: 1 },
        ),
      );
  });
  expect(client.getQueryData(["newapi-channels", "primary", 0, 50, "all"])).toBeUndefined();
  expect(client.getQueryData(["newapi-channel-groups", "primary"])).toBeUndefined();
});

it("关闭分组成员弹窗取消该弹窗独占的渠道读取", async () => {
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>(() => {});
  });
  const view = render(
    <QueryClientProvider client={createClient()}>
      <ChannelGroupMemberDialog
        platformId="primary"
        groupName="生产"
        memberIDs={[]}
        onClose={() => {}}
        onAdd={() => {}}
      />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取可添加的渠道" });
  view.unmount();
  expect(signal?.aborted).toBe(true);
});

it("关闭成员弹窗时列表仍使用同一请求，继续读取并显示列表结果", async () => {
  const user = userEvent.setup();
  let signal: AbortSignal | null | undefined;
  let resolve!: (value: Response) => void;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input).endsWith("channel-groups"))
      return Promise.resolve(Response.json({ groups: [], version: "" }));
    signal = init?.signal;
    return new Promise<Response>((complete) => {
      resolve = complete;
    });
  });
  function SharedReaders() {
    const [open, setOpen] = useState(true);
    return (
      <>
        <ExistingChannels platformId="primary" />
        {open && (
          <ChannelGroupMemberDialog
            platformId="primary"
            groupName="生产"
            memberIDs={[]}
            onClose={() => setOpen(false)}
            onAdd={() => {}}
          />
        )}
      </>
    );
  }
  renderWithRouter(
    <QueryClientProvider client={createClient()}>
      <SharedReaders />
    </QueryClientProvider>,
  );
  await screen.findByRole("status", { name: "正在读取可添加的渠道" });
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(signal?.aborted).toBe(false);
  await act(async () => resolve(Response.json({ items: [channel], total: 1 })));
  expect(await screen.findByRole("row", { name: "渠道 生产渠道（41）" })).toBeVisible();
});

it("选择整组期间离页取消整组读取且不会报告取消为业务失败", async () => {
  const user = userEvent.setup();
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname.endsWith("channel-groups"))
      return Promise.resolve(
        Response.json({
          groups: [{ id: "prod", name: "生产", channel_ids: ["41"] }],
          version: "v1",
        }),
      );
    if (url.searchParams.get("page_size") === "100") {
      signal = init?.signal;
      return new Promise<Response>(() => {});
    }
    return Promise.resolve(Response.json({ items: [channel], total: 1 }));
  });
  renderWithRouter(
    <QueryClientProvider client={createClient()}>
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
  await screen.findByRole("row", { name: "渠道 生产渠道（41）" });
  await user.click(screen.getByRole("combobox", { name: "按渠道分组筛选" }));
  await user.click(screen.getByRole("option", { name: "生产（1）" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "选择整组" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "选择整组" }));
  cleanup();
  expect(signal?.aborted).toBe(true);
  expect(
    client
      .getQueryCache()
      .getAll()
      .some((query) => query.state.status === "error"),
  ).toBe(false);
});
