import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { PricingGroup } from "@/api";
import { dictionaryItems } from "@/test/dictionary-fixtures";
import { ExchangeGroupSetEditor } from "../exchange-group-set-editor";

const group: PricingGroup = {
  id: "6",
  name: "标准分组",
  platform: "openai",
  status: "active",
  available: true,
  managed: true,
  rate_multiplier: "1",
  reason: null,
};
let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount(missing: "group" | "platform"): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const cached = missing === "group" ? "platform" : "group";
  client.setQueryData(["dictionaries", cached], {
    items: dictionaryItems(cached, [
      cached === "group" ? { value: "6", name: "标准分组" } : { value: "openai", name: "OpenAI" },
    ]),
  });
  render(
    <QueryClientProvider client={client}>
      <ExchangeGroupSetEditor
        setIndex={0}
        name="常规价格"
        groupSet={["6"]}
        groups={[group]}
        exchangeSetByGroup={new Map([["6", 0]])}
        exchangeSetNames={["常规价格"]}
        groupMinimums={{ "6": "0.25" }}
        onMinimumChange={vi.fn()}
        onToggle={vi.fn()}
        onNameChange={vi.fn()}
        onRemove={vi.fn()}
      />
    </QueryClientProvider>,
  );
}

it.each(["group", "platform"] as const)(
  "%s 字典首次读取等待时显示忙碌提示，失败后重试恢复原选择和最低倍率",
  async (kind) => {
    let finish!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      finish = resolve;
    });
    const items = dictionaryItems(kind, [
      kind === "group" ? { value: "6", name: "标准分组" } : { value: "openai", name: "OpenAI" },
    ]);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockReturnValueOnce(pending).mockResolvedValueOnce(Response.json({ items })),
    );
    mount(kind);
    expect(screen.getByRole("status", { name: "正在读取字典选项" })).toBeVisible();
    expect(screen.queryByText("暂无可选分组")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "删除互换组 1" })).toBeEnabled();

    await act(async () => finish(Response.json({ detail: "字典暂不可用" }, { status: 503 })));
    const retry = await screen.findByRole("button", { name: "重新读取" });
    expect(screen.queryByText("暂无可选分组")).not.toBeInTheDocument();
    await userEvent.setup().click(retry);

    expect(await screen.findByRole("checkbox", { name: /^互换组 1 分组 标准分组/ })).toBeChecked();
    expect(screen.getByRole("textbox", { name: "分组 标准分组 最低迁入倍率" })).toHaveValue("0.25");
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  },
);

it("已读字典后台刷新失败时保留选中分组和正在编辑的最低倍率", async () => {
  vi.stubGlobal("fetch", async () =>
    Response.json({ items: dictionaryItems("group", [{ value: "6", name: "标准分组" }]) }),
  );
  mount("group");
  const minimum = await screen.findByRole("textbox", { name: "分组 标准分组 最低迁入倍率" });
  const user = userEvent.setup();
  await user.clear(minimum);
  await user.type(minimum, "0.35");
  vi.stubGlobal("fetch", async () => Response.json({ detail: "字典暂不可用" }, { status: 503 }));
  await act(async () => {
    await client.refetchQueries({ queryKey: ["dictionaries"] });
  });
  expect(minimum).toHaveValue("0.35");
  expect(screen.getByRole("checkbox", { name: /^互换组 1 分组 标准分组/ })).toBeChecked();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});

it.each(["group", "platform"] as const)(
  "%s 字典已读取但为空时展示空态，不从已有分组补造候选项",
  async (kind) => {
    vi.stubGlobal("fetch", async () => Response.json({ items: [] }));
    mount(kind);
    expect(await screen.findByText("暂无可选分组")).toBeVisible();
    await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "互换组 1 规则名称" })).toHaveValue("常规价格");
  },
);
