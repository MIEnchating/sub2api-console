import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { PricingSnapshot } from "@/api";
import {
  PricingConfigPage,
  PricingPage,
  PricingPreviewTable,
  pricingPreviewDecisions,
} from "../pricing-page";

const snapshot: PricingSnapshot = {
  config: {
    enabled: false,
    profit_margin: 0.2,
    exchange_group_sets: [["6", "7"]],
    interval_seconds: 120,
    write_concurrency: 4,
  },
  groups: [
    {
      id: "6",
      name: "标准",
      platform: "openai",
      status: "active",
      rate_multiplier: "1",
      managed: true,
      available: true,
      reason: null,
    },
    {
      id: "7",
      name: "复合",
      platform: "composite",
      status: "active",
      rate_multiplier: "0.5",
      managed: true,
      available: false,
      reason: "复合分组由模型路由控制，不自动调整账号成员",
    },
  ],
  decisions: [
    {
      account_id: "41",
      account_name: "account-41",
      platform: "openai",
      cost_multiplier: "0.9",
      current_group_ids: ["6"],
      desired_group_ids: [],
      eligible_groups: [],
      changed: true,
      skipped: false,
      reason: null,
    },
  ],
  accounts: 1,
  changes: 1,
  skipped: 0,
  generated_at: "2026-08-30T12:00:00Z",
};

describe("PricingPage", () => {
  it("keeps the account adjustment preview with price management", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    queryClient.setQueryData(["pricing"], snapshot);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <PricingPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("价格管理");
    expect(markup).toContain("分组售价");
    expect(markup).toContain("进货倍率");
    expect(markup).toContain("售价倍率");
    expect(markup).toContain("标准");
    expect(markup).toContain(">0.9</span>");
    expect(markup).toContain("1 个账号");
    expect(markup).not.toContain("价格配置");
    expect(markup).not.toContain("目标盈利比例");
    expect(markup).toContain("查看账号调整明细");
    expect(markup).not.toContain("计算收入");
    expect(markup).not.toContain("收益分析");
  });

  it("renders automatic pricing controls only on the dedicated configuration page", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    queryClient.setQueryData(["pricing"], snapshot);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <PricingConfigPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("价格配置");
    expect(markup).toContain("默认关闭");
    expect(markup).toContain('aria-label="启用动态价格分组"');
    expect(markup).toContain("目标盈利比例");
    expect(markup).toContain('value="20"');
    expect(markup).toContain('aria-label="互换组 1 分组 标准"');
    expect(markup).toContain('aria-label="互换组 1 分组 复合"');
    expect(markup).not.toContain("查看账号调整明细");
    expect(markup).not.toContain("计算收入");
    expect(markup).toContain("账号互换范围");
    expect(markup).toContain("互换组 1");
    expect(markup).toContain('data-testid="pricing-config-page"');
    expect(markup).toContain('data-testid="pricing-settings-grid"');
    expect(markup).toContain('data-testid="pricing-page-actions"');
    expect(markup).toContain("flex-wrap");
    expect(markup).toContain('data-testid="exchange-set-1"');
    expect(markup).toContain('data-platform-section="openai"');
    expect(markup).toContain('data-selected="true"');
    expect(markup).toContain('aria-label="收起互换组 1"');
    expect(markup).toContain('aria-expanded="true"');
    expect(markup).toContain('aria-controls="exchange-set-content-1"');
    expect(markup).toContain('id="exchange-set-content-1"');
    expect(markup).toContain(">售价 1</span>");
    expect(markup).not.toContain("account-41");
    expect(markup).toContain("disabled");
  });

  it("uses a compact actionable empty state when no exchange set exists", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    queryClient.setQueryData(["pricing"], {
      ...snapshot,
      config: { ...snapshot.config, exchange_group_sets: [] },
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <PricingConfigPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain('data-testid="exchange-groups-empty"');
    expect(markup).toContain("还没有互换组");
    expect(markup).toContain("创建第一个互换组");
    expect(markup).toContain('data-slot="button"');
  });

  it("renders account changes inside the preview dialog table", () => {
    const markup = renderToStaticMarkup(
      <PricingPreviewTable
        decisions={[
          ...snapshot.decisions,
          {
            ...snapshot.decisions[0],
            account_id: "42",
            account_name: "unchanged-42",
            changed: false,
            current_group_ids: ["6"],
            desired_group_ids: ["6"],
          },
          {
            ...snapshot.decisions[0],
            account_id: "43",
            account_name: "unknown-43",
            cost_multiplier: "0",
            changed: false,
            skipped: true,
            reason: "账号成本倍率 0 无效，必须大于 0，保留原分组",
          },
        ]}
        groups={snapshot.groups}
        config={snapshot.config}
      />,
    );

    expect(markup).toContain("account-41");
    expect(markup).toContain("标准");
    expect(markup).toContain("未分组");
    expect(markup).toContain("具体变更");
    expect(markup).toContain("判定依据");
    expect(markup).toContain("#41 · openai");
    expect(markup).toContain('data-testid="pricing-preview-table"');
    expect(markup).toContain("h-full");
    expect(markup).toContain("移出：标准");
    expect(markup).toContain("进货倍率 0.9 &gt; 可接受上限 0.8");
    expect(markup).toContain("售价 1 × 80%");
    expect(markup).toContain("将调整");
    expect(markup).toContain("unchanged-42");
    expect(markup).toContain("无需调整");
    expect(markup).toContain("unknown-43");
    expect(markup).toContain("成本倍率 0 无效");
    expect(markup).toContain('aria-label="转到下一页"');
  });

  it("paginates the preview instead of rendering every account at once", () => {
    const decisions = Array.from({ length: 21 }, (_, index) => ({
      ...snapshot.decisions[0],
      account_id: String(index + 1),
      account_name: `preview-account-${index + 1}`,
    }));
    const markup = renderToStaticMarkup(
      <PricingPreviewTable
        decisions={decisions}
        groups={snapshot.groups}
        config={snapshot.config}
      />,
    );

    expect(markup).toContain("preview-account-10");
    expect(markup).not.toContain("preview-account-11");
    expect(markup).toContain('aria-label="转到第 2 页"');
    expect(markup).toContain('aria-label="转到下一页"');
  });

  it("recalculates membership from unsaved margin and managed-group changes", () => {
    const decisions = pricingPreviewDecisions(
      [
        {
          account_id: "41",
          account_name: "account-41",
          platform: "openai",
          cost_multiplier: "0.6",
          current_group_ids: ["7", "9"],
          desired_group_ids: [],
          eligible_groups: [],
          changed: false,
          skipped: false,
          reason: null,
        },
      ],
      snapshot.groups,
      { ...snapshot.config, exchange_group_sets: [["6", "7"]], profit_margin: 0.3 },
    );

    expect(decisions[0].desired_group_ids).toEqual(["6", "7", "9"]);
    expect(decisions[0].eligible_groups).toEqual(["标准"]);
    expect(decisions[0].changed).toBe(true);
  });
});
