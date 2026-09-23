import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { NewAPIChannel } from "@/api";
import { ExistingChannels } from "../existing-channels";

const channel: NewAPIChannel = {
  id: "42",
  name: "标准渠道",
  type: 59,
  status: 1,
  models: ["old"],
  groups: ["default"],
  version: "a".repeat(64),
};
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
}

it("上架时获取上游模型，选择后合并手输内容且确认前不写入渠道", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      if (String(url).includes("/42/models/available"))
        return Response.json({ models: ["old", "new-model"] });
      if (options?.method === "PUT") {
        writes.push(JSON.parse(String(options.body)));
        return Response.json(channel);
      }
      return Response.json({ items: [channel], total: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.type(screen.getByLabelText("上架模型名称"), "manual-model");
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  const picker = within(await screen.findByRole("dialog", { name: "选择上游模型" }));
  await user.click(await picker.findByRole("checkbox", { name: /new-model/ }));
  await user.click(screen.getByRole("button", { name: "确认模型" }));
  expect(screen.getByLabelText("上架模型名称")).toHaveValue("manual-model\nnew-model");
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  expect(writes).toEqual([]);
  await user.click(screen.getByRole("button", { name: "确认上架模型" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { action: "add", models: ["manual-model", "new-model"], version: channel.version },
    ]),
  );
});

it("获取等待时禁止预览但可取消，失败后允许重新读取且保留手输模型", async () => {
  let complete: (response: Response) => void = () => undefined;
  const pending = new Promise<Response>((resolve) => {
    complete = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: RequestInfo | URL) =>
      String(url).includes("/42/models/available")
        ? pending
        : Promise.resolve(Response.json({ items: [channel], total: 1 })),
    ),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.type(screen.getByLabelText("上架模型名称"), "manual");
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  expect(await screen.findByRole("status", { name: "正在从上游获取模型" })).toBeVisible();
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
  complete(Response.json({ detail: "模型接口暂不可用，请稍后重试" }, { status: 502 }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  await user.click(screen.getAllByRole("button", { name: "取消" }).at(-1)!);
  expect(screen.getByLabelText("上架模型名称")).toHaveValue("manual");
});

it("批量上架只列出所有渠道共同支持且仍需新增的模型", async () => {
  const second = { ...channel, id: "43", name: "第二渠道", models: ["old", "common"] };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      if (String(url).includes("/42/models/available"))
        return Response.json({ models: ["old", "common", "first-only"] });
      if (String(url).includes("/43/models/available"))
        return Response.json({ models: ["old", "common", "second-only"] });
      return Response.json({ items: [channel, second], total: 2 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "全选当前筛选渠道" }));
  await user.click(screen.getByRole("button", { name: "批量上架模型" }));
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  const picker = within(await screen.findByRole("dialog", { name: "选择上游模型" }));
  const checkbox = await picker.findByRole("checkbox", { name: /common/ });
  expect(picker.getAllByRole("checkbox")).toHaveLength(1);
  checkbox.focus();
  await user.keyboard(" ");
  expect(checkbox).toBeChecked();
  await user.click(picker.getByRole("button", { name: "确认模型" }));
  expect(screen.getByLabelText("上架模型名称")).toHaveValue("common");
});

it("获取的模型均已上架时显示空结果且不能确认", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) =>
      String(url).includes("/models/available")
        ? Response.json({ models: ["old"] })
        : Response.json({ items: [channel], total: 1 }),
    ),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  expect(await screen.findByText("没有可新增的上游模型")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
});

it("读取期间取消选择后不应用晚到结果并可重新获取", async () => {
  let complete: (response: Response) => void = () => undefined;
  const pending = new Promise<Response>((resolve) => {
    complete = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: RequestInfo | URL) =>
      String(url).includes("/models/available")
        ? pending
        : Promise.resolve(Response.json({ items: [channel], total: 1 })),
    ),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "上架模型" }));
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await screen.findByRole("status", { name: "正在从上游获取模型" });
  await user.click(screen.getAllByRole("button", { name: "取消" }).at(-1)!);
  complete(Response.json({ models: ["late-model"] }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "选择上游模型" })).not.toBeInTheDocument(),
  );
  expect(screen.getByLabelText("上架模型名称")).toHaveValue("");
  expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled();
});
