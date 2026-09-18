import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { NewAPIChannel } from "@/api";
import { ExistingChannels } from "../existing-channels";

const channel: NewAPIChannel = {
  id: "42",
  name: "标准渠道",
  type: 59,
  status: 1,
  models: ["gpt-5", "gpt-5-mini"],
  groups: ["default"],
  version: "a".repeat(64),
};
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
let client: QueryClient;
function mount(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("上架模型先预览去重后的新增范围，确认后按稳定 ID 和版本提交", async () => {
  const writes: unknown[] = [];
  const fetch = vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
    if (options?.method === "PUT") {
      expect(String(url)).toContain("/channels/42/models");
      writes.push(JSON.parse(String(options.body)));
      return Response.json(channel);
    }
    return Response.json({ items: [channel], total: 1 });
  });
  vi.stubGlobal("fetch", fetch);
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.type(screen.getByLabelText("上架模型名称"), "gpt-5,gpt-5-nano\ngpt-5-nano");
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  expect(screen.getByRole("list", { name: "变更模型" })).toHaveTextContent("gpt-5-nano");
  expect(screen.getAllByRole("listitem")).toHaveLength(1);
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "确认上架模型" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(writes).toEqual([{ action: "add", models: ["gpt-5-nano"], version: channel.version }]);
});

it("下架模型支持键盘选择，禁止清空渠道，确认后只提交选中模型", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, options?: RequestInit) => {
      if (options?.method === "PUT") {
        writes.push(JSON.parse(String(options.body)));
        return Response.json(channel);
      }
      return Response.json({ items: [channel], total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "下架模型" }));
  expect(screen.getByRole("button", { name: "预览变更" })).toBeDisabled();
  const control = screen.getByRole("checkbox", { name: "gpt-5" });
  control.focus();
  await user.keyboard(" ");
  expect(control).toBeChecked();
  await user.click(screen.getByRole("checkbox", { name: "gpt-5-mini" }));
  expect(screen.getByRole("button", { name: "预览变更" })).toBeDisabled();
  await user.click(control);
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  await user.click(screen.getByRole("button", { name: "确认下架模型" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { action: "remove", models: ["gpt-5-mini"], version: channel.version },
    ]),
  );
});

it("模型变更待确认时禁止重复提交，失败后关闭旧预览并重新读取渠道", async () => {
  let complete: (response: Response) => void = () => undefined;
  const pending = new Promise<Response>((resolve) => {
    complete = resolve;
  });
  const fetch = vi.fn((_url: RequestInfo | URL, options?: RequestInit) =>
    options?.method === "PUT"
      ? pending
      : Promise.resolve(Response.json({ items: [channel], total: 1 })),
  );
  vi.stubGlobal("fetch", fetch);
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.type(screen.getByLabelText("上架模型名称"), "gpt-5-nano");
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  await user.click(screen.getByRole("button", { name: "确认上架模型" }));
  expect(screen.getByRole("button", { name: "正在提交…" })).toBeDisabled();
  complete(Response.json({ detail: "配置已变化，请刷新" }, { status: 409 }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() =>
    expect(
      fetch.mock.calls.filter(
        (call) =>
          call[1]?.method !== "PUT" &&
          new URL(String(call[0]), "http://localhost").pathname.endsWith("/channels"),
      ),
    ).toHaveLength(2),
  );
  expect(fetch.mock.calls.filter((call) => call[1]?.method === "PUT")).toHaveLength(1);
});

it("首次渠道列表读取失败显示重试，成功空列表显示空状态", async () => {
  let failed = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      failed
        ? Response.json({ detail: "读取失败" }, { status: 500 })
        : Response.json({ items: [], total: 0 }),
    ),
  );
  mount();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("button", { name: "上架模型" })).not.toBeInTheDocument();
  failed = false;
  await userEvent.setup().click(retry);
  expect(await screen.findByText("暂无渠道")).toBeVisible();
});

it("上架输入空白或超长模型时显示字段错误且不能进入确认", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items: [channel], total: 1 })),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("请输入要上架的模型名称");
  expect(screen.getByLabelText("上架模型名称")).toHaveAttribute("aria-invalid", "true");
  await user.click(screen.getByLabelText("上架模型名称"));
  await user.paste("x".repeat(256));
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("模型名称不能超过 255 个字符");
  expect(screen.queryByRole("button", { name: "确认上架模型" })).not.toBeInTheDocument();
});

it("首次读取使用骨架，后台刷新时保留已有渠道并暂时禁用变更", async () => {
  let complete: (response: Response) => void = () => undefined;
  let pending = new Promise<Response>((resolve) => {
    complete = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(() => pending),
  );
  mount();
  expect(screen.getByRole("status", { name: "正在读取现有渠道" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  complete(Response.json({ items: [channel], total: 1 }));
  expect(await screen.findByRole("row", { name: "渠道 标准渠道（42）" })).toBeVisible();
  pending = new Promise<Response>((resolve) => {
    complete = resolve;
  });
  await userEvent.setup().click(screen.getByRole("button", { name: "刷新渠道列表" }));
  expect(screen.getByRole("row", { name: "渠道 标准渠道（42）" })).toBeVisible();
  expect(screen.getByRole("button", { name: "上架模型" })).toBeDisabled();
  expect(screen.queryByRole("status", { name: "正在读取现有渠道" })).not.toBeInTheDocument();
  complete(Response.json({ items: [channel], total: 1 }));
  await waitFor(() => expect(screen.getByRole("button", { name: "上架模型" })).toBeEnabled());
});

it("超过一页的渠道可按远端页码读取，末页禁用下一页", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      const next = new URL(String(url), "http://localhost").searchParams.get("page") === "1";
      return Response.json({
        items: [{ ...channel, id: next ? "43" : "42", name: next ? "下一页渠道" : "标准渠道" }],
        total: 51,
      });
    }),
  );
  mount();
  const next = await screen.findByRole("button", { name: "转到下一页" });
  await userEvent.setup().click(next);
  expect(await screen.findByRole("row", { name: "渠道 下一页渠道（43）" })).toBeVisible();
  expect(screen.queryByRole("row", { name: "渠道 标准渠道（42）" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeEnabled();
});
