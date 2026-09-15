import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PricingConfig, PricingSnapshot, Task } from "@/api";
import { PricingConfigPage } from "../pricing-page";

const clients: QueryClient[] = [];

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup(
  minimums?: Record<string, string>,
  rejectSave = false,
): {
  user: ReturnType<typeof userEvent.setup>;
  saves: PricingConfig[];
  writes: string[];
} {
  let snapshot: PricingSnapshot = {
    config: {
      enabled: true,
      profit_margin: 0.2,
      interval_seconds: 120,
      write_concurrency: 4,
      exchange_group_sets: [["6", "7", "8"]],
      exchange_group_set_names: ["标准渠道"],
      group_min_cost_multipliers: minimums,
    },
    groups: [
      { id: "6", name: "原售价", rate_multiplier: "0.3" },
      { id: "7", name: "特价", rate_multiplier: "0.15" },
      { id: "8", name: "标准", rate_multiplier: "0.2" },
    ].map((group) => ({
      ...group,
      platform: "openai",
      status: "active",
      managed: true,
      available: true,
      reason: null,
    })),
    decisions: [
      {
        account_id: "41",
        account_name: "标准渠道账号",
        platform: "openai",
        cost_multiplier: "0.1",
        current_group_ids: ["6"],
        desired_group_ids: ["7"],
        eligible_groups: ["特价"],
        changed: true,
        skipped: false,
        reason: null,
      },
    ],
    accounts: 1,
    changes: 1,
    skipped: 0,
    generated_at: "2026-09-14T00:00:00Z",
  };
  const task: Task = {
    id: "pricing-1",
    skill: "console",
    operation: "price-group-allocation",
    status: "succeeded",
    progress: 100,
    message: "已完成",
    result: {},
    created_at: snapshot.generated_at,
    updated_at: snapshot.generated_at,
  };
  const saves: PricingConfig[] = [];
  const writes: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (init?.method === "PUT" || init?.method === "POST") writes.push(path);
      if (init?.method === "PUT") {
        if (rejectSave)
          return new Response(JSON.stringify({ error: "价格配置暂时无法保存" }), { status: 500 });
        const config = JSON.parse(String(init.body)) as PricingConfig;
        saves.push(config);
        snapshot = { ...snapshot, config };
      }
      return new Response(
        JSON.stringify(path.includes("/apply") || path.includes("/tasks/") ? task : snapshot),
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["pricing"], snapshot);
  render(
    <QueryClientProvider client={client}>
      <PricingConfigPage />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), saves, writes };
}

describe("最低迁入倍率设置", () => {
  it("编辑已选分组倍率并保存时保留十进制原始精度且不改变分组选择", async () => {
    const view = setup();
    const input = screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" });
    await view.user.type(input, "0.10000000000000001");
    await view.user.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(view.saves).toHaveLength(1));
    expect(view.saves[0].group_min_cost_multipliers).toEqual({ "7": "0.10000000000000001" });
    expect(screen.getByRole("checkbox", { name: /^互换组 1 分组 特价/ })).toBeChecked();
  });

  it("输入负倍率时显示字段错误并禁止保存和立即调整", async () => {
    const view = setup();
    const input = screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" });
    await view.user.type(input, "-1");
    expect(await screen.findByRole("alert")).toHaveTextContent("请输入有效的非负倍率");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "立即调整" })).toBeDisabled();
  });

  it("非法倍率修正为零后移除错误并允许保存", async () => {
    const view = setup();
    const input = screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" });
    await view.user.type(input, "-1");
    await screen.findByRole("alert");
    await view.user.clear(input);
    await view.user.type(input, "0");
    await waitFor(() => expect(input).toHaveAttribute("aria-invalid", "false"));
    await view.user.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(view.saves[0]?.group_min_cost_multipliers).toEqual({ "7": "0" }));
  });

  it("清空已保存的倍率后保存时移除限制", async () => {
    const view = setup({ "7": "0.11" });
    await view.user.clear(screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" }));
    await view.user.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(view.saves).toHaveLength(1));
    expect(view.saves[0].group_min_cost_multipliers).toBeUndefined();
  });

  it("仅修改最低迁入倍率后立即调整时先预览其他合适分组并保存新限制", async () => {
    const view = setup();
    await view.user.type(screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" }), "0.11");
    await view.user.click(screen.getByRole("button", { name: "立即调整" }));
    expect(screen.getByRole("dialog", { name: "确认价格分组调整" })).toBeInTheDocument();
    expect(screen.getByText("标准（#8）")).toBeInTheDocument();
    expect(view.writes).toEqual([]);
    await view.user.click(screen.getByRole("button", { name: "确认调整" }));
    await waitFor(() => expect(view.writes).toEqual(["/api/pricing/config", "/api/pricing/apply"]));
    expect(view.saves[0].group_min_cost_multipliers).toEqual({ "7": "0.11" });
  });

  it("取消分组选择后再次选中时清除原有下限并保留其他分组下限", async () => {
    const view = setup({ "6": "0.01", "7": "0.11", "8": "0.02" });
    const checkbox = screen.getByRole("checkbox", { name: /^互换组 1 分组 特价/ });
    checkbox.focus();
    await view.user.keyboard(" ");
    expect(
      screen.queryByRole("textbox", { name: "分组 特价 最低迁入倍率" }),
    ).not.toBeInTheDocument();
    await view.user.keyboard(" ");
    expect(screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" })).toHaveValue("");
    await view.user.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() =>
      expect(view.saves[0]?.group_min_cost_multipliers).toEqual({ "6": "0.01", "8": "0.02" }),
    );
  });

  it("保存倍率失败时保留输入且不启动价格调整任务", async () => {
    const view = setup(undefined, true);
    await view.user.type(screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" }), "0.11");
    await view.user.click(screen.getByRole("button", { name: "立即调整" }));
    await view.user.click(screen.getByRole("button", { name: "确认调整" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "确认调整" })).toBeEnabled());
    expect(view.writes).toEqual(["/api/pricing/config"]);
    await view.user.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.getByRole("textbox", { name: "分组 特价 最低迁入倍率" })).toHaveValue("0.11");
  });

  it("删除互换组后保存时一并删除该互换组的倍率限制", async () => {
    const view = setup({ "7": "0.11" });
    await view.user.click(screen.getByRole("switch", { name: "启用动态价格分组" }));
    await view.user.click(screen.getByRole("button", { name: "删除互换组 1" }));
    await view.user.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(view.saves).toHaveLength(1));
    expect(view.saves[0].exchange_group_sets).toEqual([]);
    expect(view.saves[0].group_min_cost_multipliers).toBeUndefined();
  });
});
