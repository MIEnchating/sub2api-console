import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { NewAPIChannel } from "@/api";
import { ExistingChannels } from "../existing-channels";

const items: NewAPIChannel[] = [
  {
    id: "41",
    name: "生产渠道",
    type: 59,
    status: 1,
    groups: ["default"],
    models: ["gpt-5"],
    version: "a".repeat(64),
  },
];

let client: QueryClient;

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
}

it("创建分组并把已选渠道加入后保存到后端", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      if (String(url).endsWith("/channel-groups") && options?.method === "PUT") {
        const payload = JSON.parse(String(options.body)) as unknown;
        writes.push(payload);
        return Response.json({
          groups: [{ id: "group-prod", name: "生产渠道", channel_ids: ["41"] }],
          version: "next",
        });
      }
      if (String(url).endsWith("/channel-groups")) {
        return Response.json({ groups: [], version: "" });
      }
      return Response.json({ items, total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（41）" });
  await user.click(screen.getByRole("checkbox", { name: "选择渠道 生产渠道（41）" }));
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.type(screen.getByRole("textbox", { name: "新建分组名称" }), "生产渠道");
  await user.click(screen.getByRole("button", { name: "新建分组" }));
  const groupName = screen.getByRole("textbox", { name: "分组名称 1" });
  await user.clear(groupName);
  await user.type(groupName, "生产主渠道");
  await user.click(screen.getByRole("button", { name: "加入已选" }));
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    groups: [{ id: expect.stringMatching(/^group-/), name: "生产主渠道", channel_ids: ["41"] }],
    version: "",
  });
});

it("选择保存的分组后读取跨页成员，并一次选中整组的最新渠道", async () => {
  const member = { ...items[0], id: "101", name: "跨页渠道" };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      const path = new URL(String(url), "http://localhost");
      if (path.pathname.endsWith("/channel-groups"))
        return Response.json({
          groups: [{ id: "prod", name: "生产", channel_ids: ["101"] }],
          version: "v1",
        });
      if (path.searchParams.get("group_id") === "prod")
        return Response.json({ items: [member], total: 1 });
      return Response.json({ items, total: 101 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（41）" });
  const filter = screen.getByRole("combobox", { name: "按渠道分组筛选" });
  expect(filter).toHaveTextContent("全部渠道分组");
  await user.click(filter);
  await user.click(screen.getByRole("option", { name: "生产（1）" }));
  await screen.findByRole("row", { name: "渠道 跨页渠道（101）" });
  expect(screen.queryByRole("row", { name: "渠道 生产渠道（41）" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "选择整组" }));
  await waitFor(() =>
    expect(screen.getByRole("checkbox", { name: "选择渠道 跨页渠道（101）" })).toBeChecked(),
  );
});

it("删除最后一个分组后仍可保存空列表", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      if (String(url).endsWith("/channel-groups")) {
        if (options?.method === "PUT") {
          writes.push(JSON.parse(String(options.body)));
          return Response.json({ groups: [], version: "v2" });
        }
        return Response.json({
          groups: [{ id: "prod", name: "生产", channel_ids: ["41"] }],
          version: "v1",
        });
      }
      return Response.json({ items, total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（41）" });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(await screen.findByRole("button", { name: "删除分组 生产" }));
  expect(screen.getByText("暂无渠道分组")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() => expect(writes).toEqual([{ groups: [], version: "v1" }]));
});

it("移除已选成员只改变该组，并可按 ID 移除已不存在的渠道", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      if (String(url).endsWith("/channel-groups")) {
        if (options?.method === "PUT") {
          const payload = JSON.parse(String(options.body)) as {
            groups: unknown[];
            version: string;
          };
          writes.push(payload);
          return Response.json({ ...payload, version: "v2" });
        }
        return Response.json({
          groups: [{ id: "prod", name: "生产", channel_ids: ["41", "99"] }],
          version: "v1",
        });
      }
      return Response.json({ items, total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "选择渠道 生产渠道（41）" }));
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(await screen.findByRole("button", { name: "移除已选" }));
  await user.click(screen.getByText("1 个渠道", { selector: "summary" }));
  await user.click(screen.getByRole("button", { name: "从 生产 移除渠道 99" }));
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { groups: [{ id: "prod", name: "生产", channel_ids: [] }], version: "v1" },
    ]),
  );
});

it("保存冲突后保留草稿和原版本，关闭重开才读取新版本", async () => {
  let reads = 0;
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      if (String(url).endsWith("/channel-groups")) {
        if (options?.method === "PUT") {
          writes.push(JSON.parse(String(options.body)));
          return Response.json({ detail: "渠道分组已修改" }, { status: 409 });
        }
        reads++;
        return Response.json({
          groups: [{ id: "prod", name: reads === 1 ? "生产" : "远端名称", channel_ids: [] }],
          version: reads === 1 ? "v1" : "v2",
        });
      }
      return Response.json({ items, total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（41）" });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  const name = await screen.findByRole("textbox", { name: "分组名称 1" });
  await user.clear(name);
  await user.type(name, "草稿");
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() => expect(reads).toBe(2));
  expect(name).toHaveValue("草稿");
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes[1]).toMatchObject({ version: "v1" });
  await user.click(screen.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  expect(await screen.findByRole("textbox", { name: "分组名称 1" })).toHaveValue("远端名称");
});

it("分组读取失败时仍能取消或重试，未读取到配置不能保存", async () => {
  let failed = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      if (String(url).endsWith("/channel-groups"))
        return failed
          ? Response.json({ detail: "读取失败" }, { status: 503 })
          : Response.json({ groups: [], version: "" });
      return Response.json({ items, total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("button", { name: "重试渠道分组" });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  const dialog = screen.getByRole("dialog", { name: "渠道分组" });
  expect(within(dialog).getByRole("button", { name: "取消" })).toBeEnabled();
  expect(within(dialog).queryByRole("button", { name: "保存分组" })).not.toBeInTheDocument();
  failed = false;
  await user.click(within(dialog).getByRole("button", { name: "重新读取" }));
  expect(await screen.findByRole("button", { name: "保存分组" })).toBeEnabled();
});
