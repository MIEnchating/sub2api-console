import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { NewAPIChannel, NewAPIChannelGroups } from "@/api";
import { ExistingChannels } from "../existing-channels";

const primary: NewAPIChannel = {
  id: "1",
  name: "生产渠道",
  type: 59,
  status: 1,
  groups: ["default"],
  models: ["gpt-5"],
  version: "v1",
};
const backup: NewAPIChannel = { ...primary, id: "2", name: "备用渠道" };
const remote: NewAPIChannel = { ...primary, id: "51", name: "跨页渠道" };
let client: QueryClient;
let writes: NewAPIChannelGroups[];
let failPicker: boolean;
let emptyPicker: boolean;

beforeEach(() => {
  writes = [];
  failPicker = false;
  emptyPicker = false;
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      const path = new URL(String(url), "http://localhost");
      if (path.pathname.endsWith("/channel-groups")) {
        if (options?.method === "PUT") {
          const payload = JSON.parse(String(options.body)) as NewAPIChannelGroups;
          writes.push(payload);
          return Response.json({ ...payload, version: "v2" });
        }
        return Response.json({
          groups: [{ id: "prod", name: "生产", channel_ids: ["1"] }],
          version: "v1",
        });
      }
      if (failPicker) return Response.json({ detail: "渠道读取失败" }, { status: 503 });
      if (emptyPicker) return Response.json({ items: [], total: 0 });
      if (path.searchParams.get("page") === "1")
        return Response.json({ items: [remote], total: 51 });
      return Response.json({ items: [primary, backup], total: 51 });
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
});

afterEach(() => {
  cleanup();
  client.clear();
  vi.unstubAllGlobals();
});

it("未在列表勾选时，可在分组弹窗搜索并跨页添加渠道，保存时只提交稳定 ID", async () => {
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（1）" });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(screen.getByRole("button", { name: "添加渠道" }));
  const picker = screen.getByRole("dialog", { name: "添加渠道到分组" });
  expect(
    await within(picker).findByRole("checkbox", { name: "选择渠道 生产渠道（1）" }),
  ).toHaveAttribute("aria-disabled", "true");
  const search = within(picker).getByRole("textbox", { name: "搜索本页渠道名称或 ID" });
  await user.type(search, "备用");
  await user.click(within(picker).getByRole("checkbox", { name: "选择渠道 备用渠道（2）" }));
  await user.click(within(picker).getByRole("button", { name: "转到下一页" }));
  await user.click(
    await within(picker).findByRole("checkbox", { name: "选择渠道 跨页渠道（51）" }),
  );
  await user.click(within(picker).getByRole("button", { name: "添加到分组（2）" }));
  await waitFor(() => expect(picker).not.toBeInTheDocument());
  expect(writes).toHaveLength(0);
  await user.click(screen.getByText("3 个渠道", { selector: "summary" }));
  expect(screen.getByText("备用渠道", { selector: "span" })).toBeVisible();
  expect(screen.getByText("跨页渠道", { selector: "span" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { groups: [{ id: "prod", name: "生产", channel_ids: ["1", "2", "51"] }], version: "v1" },
    ]),
  );
});

it("取消弹窗内的渠道选择时，不修改分组成员", async () => {
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（1）" });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(screen.getByRole("button", { name: "添加渠道" }));
  const picker = screen.getByRole("dialog", { name: "添加渠道到分组" });
  await user.click(await within(picker).findByRole("checkbox", { name: "选择渠道 备用渠道（2）" }));
  await user.click(within(picker).getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() => expect(writes[0].groups[0].channel_ids).toEqual(["1"]));
});

it("渠道读取失败时提供重试，重试成功后可以选择渠道", async () => {
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（1）" });
  failPicker = true;
  client.removeQueries({ queryKey: ["newapi-channels"] });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(screen.getByRole("button", { name: "添加渠道" }));
  const picker = screen.getByRole("dialog", { name: "添加渠道到分组" });
  const retry = await within(picker).findByRole("button", { name: "重新读取" });
  expect(within(picker).getByRole("button", { name: "添加到分组（0）" })).toBeDisabled();
  expect(within(picker).getByRole("button", { name: "取消" })).toBeEnabled();
  failPicker = false;
  await user.click(retry);
  expect(
    await within(picker).findByRole("checkbox", { name: "选择渠道 备用渠道（2）" }),
  ).toBeEnabled();
});

it("渠道目录为空时显示空状态且不能添加", async () => {
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（1）" });
  emptyPicker = true;
  client.removeQueries({ queryKey: ["newapi-channels"] });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(screen.getByRole("button", { name: "添加渠道" }));
  const picker = screen.getByRole("dialog", { name: "添加渠道到分组" });
  expect(await within(picker).findByText("暂无可添加的渠道")).toBeVisible();
  expect(within(picker).getByRole("button", { name: "添加到分组（0）" })).toBeDisabled();
});

it("翻页读取期间显示加载提示并保留跨页选择，取消仍可用", async () => {
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 生产渠道（1）" });
  await user.click(screen.getByRole("button", { name: "渠道分组" }));
  await user.click(screen.getByRole("button", { name: "添加渠道" }));
  const picker = screen.getByRole("dialog", { name: "添加渠道到分组" });
  await user.click(await within(picker).findByRole("checkbox", { name: "选择渠道 备用渠道（2）" }));
  let resolvePage!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    resolvePage = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(() => response),
  );
  await user.click(within(picker).getByRole("button", { name: "转到下一页" }));
  expect(within(picker).getByRole("status", { name: "正在读取可添加的渠道" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(within(picker).getByRole("button", { name: "取消" })).toBeEnabled();
  expect(within(picker).getByRole("button", { name: "添加到分组（1）" })).toBeDisabled();
  resolvePage(Response.json({ items: [remote], total: 51 }));
  expect(
    await within(picker).findByRole("checkbox", { name: "选择渠道 跨页渠道（51）" }),
  ).toBeVisible();
  await waitFor(() =>
    expect(within(picker).getByRole("button", { name: "添加到分组（1）" })).toBeEnabled(),
  );
});
