import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PricingSnapshot } from "@/api";
import { PricingConfigPage } from "../pricing-page";

let client: QueryClient;

function renderConfig(groups?: PricingSnapshot["groups"]): void {
  const snapshot: PricingSnapshot = {
    config: {
      enabled: false,
      profit_margin: 0.2,
      interval_seconds: 120,
      write_concurrency: 4,
      exchange_group_sets: [["6", "7"]],
      exchange_group_set_names: ["常规价格"],
    },
    groups:
      groups ??
      [
        { id: "6", name: "平价", available: true },
        { id: "7", name: "特价", available: true },
        { id: "8", name: "停用分组", available: false },
      ].map((group) => ({
        ...group,
        platform: "openai",
        status: "active",
        managed: true,
        rate_multiplier: "0.2",
        reason: group.available ? null : "分组已停用",
      })),
    decisions: [],
    accounts: 0,
    changes: 0,
    skipped: 0,
    generated_at: "2026-09-15T00:00:00Z",
  };
  client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["pricing"], snapshot);
  client.setQueryData(["dictionaries", "group"], {
    items: ["8", "7", "6"].map((value) => ({ value, enabled: true })),
  });
  render(
    <QueryClientProvider client={client}>
      <PricingConfigPage />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

describe("价格配置互换范围布局", () => {
  it("已选与未选分组同行展示时卡片按内容高度排列", () => {
    renderConfig();
    expect(
      screen
        .getByTestId("exchange-set-options-1")
        .querySelector('[data-slot="exchange-group-grid"]'),
    ).toHaveClass("items-start");
  });

  it("规则名称与操作在首行展示，选择摘要独立换行", () => {
    renderConfig();
    const rule = screen.getByTestId("exchange-set-1");
    const header = rule.querySelector('[data-slot="exchange-set-heading"]');
    const summary = rule.querySelector('[data-slot="exchange-set-summary"]');
    expect(header).toHaveClass("grid-cols-[auto_minmax(0,1fr)_auto]");
    expect(header).toContainElement(screen.getByRole("textbox", { name: "互换组 1 规则名称" }));
    expect(header).toContainElement(screen.getByRole("button", { name: "收起互换组 1" }));
    expect(summary).toHaveTextContent("2 个分组");
    expect(summary).toHaveTextContent("openai");
  });

  it("分组展示稳定 ID 和售价，停用分组仍可读取原因", () => {
    renderConfig();
    const selected = screen.getByRole("checkbox", { name: /^互换组 1 分组 平价/ });
    const card = selected.closest('[data-slot="exchange-group-card"]');
    expect(card).toHaveTextContent("#6");
    expect(card).toHaveTextContent("售价 0.2");
    const unavailable = screen.getByRole("checkbox", { name: /^互换组 1 分组 停用分组/ });
    expect(unavailable).toHaveAttribute("aria-disabled", "true");
    expect(unavailable).toHaveAccessibleDescription("分组已停用");
  });

  it("分组列表为空时显示可执行的引导并保留删除规则入口", () => {
    renderConfig([]);
    const options = screen.getByRole("group", { name: "互换组 1 可选分组" });
    expect(within(options).getByText("暂无可选分组")).toBeVisible();
    expect(within(options).getByText(/请先在分组管理中/)).toBeVisible();
    expect(screen.getByRole("button", { name: "删除互换组 1" })).toBeEnabled();
  });

  it("键盘折叠并重新展开规则后保留最低倍率及选中状态", async () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    const user = userEvent.setup();
    renderConfig();
    await user.type(screen.getByRole("textbox", { name: "分组 平价 最低迁入倍率" }), "0.15");
    screen.getByRole("button", { name: "收起互换组 1" }).focus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "展开互换组 1" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.queryByRole("checkbox", { name: /^互换组 1 分组 平价/ })).not.toBeInTheDocument();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "收起互换组 1" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByRole("checkbox", { name: /^互换组 1 分组 平价/ })).toBeChecked();
    expect(screen.getByRole("textbox", { name: "分组 平价 最低迁入倍率" })).toHaveValue("0.15");
  });
});

it("字典将停用分组置前时互换组保留字典顺序和选中状态", () => {
  renderConfig();
  const options = screen.getAllByRole("checkbox");
  expect(options[0]).toHaveAccessibleName(/^互换组 1 分组 停用分组/);
  expect(options[1]).toHaveAccessibleName(/^互换组 1 分组 特价/);
  expect(options[2]).toHaveAccessibleName(/^互换组 1 分组 平价/);
  expect(options[0]).not.toBeChecked();
  expect(options[1]).toBeChecked();
  expect(options[2]).toBeChecked();
});

it("互换组平台分区按平台字典排列，每个平台内保留分组字典顺序", () => {
  renderConfig(
    [
      { id: "6", name: "A", platform: "openai" },
      { id: "7", name: "B", platform: "openai" },
      { id: "8", name: "C", platform: "anthropic" },
    ].map((group) => ({
      ...group,
      available: true,
      managed: true,
      status: "active",
      rate_multiplier: "1",
      reason: null,
    })),
  );
  // Clear the selected scope to display all platforms, then apply dictionary order.
  const current = client.getQueryData<PricingSnapshot>(["pricing"])!;
  act(() => {
    client.setQueryData(["pricing"], {
      ...current,
      config: { ...current.config, exchange_group_sets: [[]] },
    });
    client.setQueryData(["dictionaries", "platform"], {
      items: [
        { value: "anthropic", enabled: true },
        { value: "openai", enabled: true },
      ],
    });
  });
  return waitFor(() => {
    const options = screen.getAllByRole("checkbox");
    expect(options[0]).toHaveAccessibleName(/^互换组 1 分组 C/);
    expect(options[1]).toHaveAccessibleName(/^互换组 1 分组 B/);
    expect(options[2]).toHaveAccessibleName(/^互换组 1 分组 A/);
  });
});
