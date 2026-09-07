import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AccountCreationSettings, GroupStatus } from "@/api";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";

describe("账号设置响应式布局", () => {
  it("宽屏将模型与路由策略分栏，窄屏仍按顺序纵向排列", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const settings: AccountCreationSettings = {
      default: {
        models: [],
        concurrency: 10,
        load_factor: null,
        priority: 1,
        pool_mode: false,
        pool_mode_retry_count: 3,
        pool_mode_retry_status_codes: [401, 403, 429],
      },
      groups: [],
      platform_probe_models: {},
    };
    queryClient.setQueryData(["account-creation-settings"], settings);
    queryClient.setQueryData(["groups"], []);

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
      </QueryClientProvider>,
    );
    const gridStart = markup.indexOf('data-testid="account-creation-routing-grid"');
    const gridElementStart = markup.lastIndexOf("<div", gridStart);
    const gridTag = markup.slice(gridElementStart, markup.indexOf(">", gridStart));
    const formStart = markup.indexOf('data-testid="account-creation-policy-layout"');
    const formElementStart = markup.lastIndexOf("<form", formStart);
    const formTag = markup.slice(formElementStart, markup.indexOf(">", formStart));

    expect(gridStart).toBeGreaterThan(-1);
    expect(gridElementStart).toBeGreaterThan(-1);
    expect(gridTag).toContain("grid");
    expect(gridTag).toContain("sm:grid-cols-3");
    expect(gridTag).not.toContain("grid-cols-3 ");
    expect(formStart).toBeGreaterThan(-1);
    const document = new DOMParser().parseFromString(markup, "text/html");
    expect(
      Array.from(document.querySelector('[data-slot="settings-scroll"]')?.classList ?? []),
    ).toEqual(expect.arrayContaining(["grid", "lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]"]));
    expect(formTag).not.toContain("sm:grid-cols-2");
    expect(markup).toContain("field-sizing-fixed");
    expect(markup).toContain("h-44");
    expect(markup).toContain("resize-none");
    expect(markup).toContain('aria-label="负载因子说明"');
    expect(markup).toContain('aria-label="优先级说明"');
    expect(markup).toContain('aria-label="池模式说明"');
    expect(markup).not.toContain(">留空时跟随并发<");
    expect(markup).not.toContain(">数值越小越优先<");
  });

  it("窄屏分组摘要保持可收缩截断且不挤压独立设置开关", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const group: GroupStatus = {
      id: "6",
      name: "用于验证超长名称不会挤压开关的账号分组",
      account_count: 0,
      scheduling_open: 0,
      scheduling_closed: 0,
      scheduling_unknown: 0,
      strategy: "balanced",
      strategy_source: "global_default",
      participation_status: "participating",
      participation_reason: null,
      status: "empty",
    };
    const policy = {
      models: ["gpt-5.6-sol"],
      concurrency: 64,
      load_factor: "32",
      priority: 10,
      pool_mode: true,
      pool_mode_retry_count: 3,
      pool_mode_retry_status_codes: [429, 503],
    };
    queryClient.setQueryData(["account-creation-settings"], {
      default: policy,
      groups: [{ group_id: "6", ...policy }],
      platform_probe_models: {},
    });
    queryClient.setQueryData(["groups"], [group]);

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountCreationSettingsCard
          fallbackConcurrency={10}
          fallbackPriority={1}
          initialScope="6"
        />
      </QueryClientProvider>,
    );
    const rowAttribute = markup.indexOf('data-testid="account-group-settings-6"');
    const rowStart = markup.lastIndexOf("<div", rowAttribute);
    const rowEnd = markup.indexOf(' id="account-group-settings-6"', rowAttribute);
    const rowHeader = markup.slice(rowStart, rowEnd);
    const cardIndex = markup.indexOf('data-testid="account-creation-settings-card"');
    const cardStart = markup.lastIndexOf("<div", cardIndex);
    const cardTag = markup.slice(cardStart, markup.indexOf(">", cardIndex));

    expect(rowAttribute).toBeGreaterThan(-1);
    expect(rowStart).toBeGreaterThan(-1);
    expect(rowEnd).toBeGreaterThan(rowStart);
    expect(rowHeader).toContain("min-w-0 flex-1");
    expect(rowHeader.match(/block truncate/g)).toHaveLength(2);
    expect(rowHeader).toContain('aria-label="用于验证超长名称不会挤压开关的账号分组 使用独立设置"');
    expect(cardIndex).toBeGreaterThan(-1);
    expect(cardTag).toContain("h-full");
    expect(markup).toContain("分组独立配置");
  });

  it("分组展开按钮禁用全局按压缩放动画", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const policy = {
      models: [],
      concurrency: 10,
      load_factor: null,
      priority: 1,
      pool_mode: false,
      pool_mode_retry_count: 3,
      pool_mode_retry_status_codes: [401, 403, 429],
    };
    queryClient.setQueryData(["account-creation-settings"], {
      default: policy,
      groups: [],
      platform_probe_models: {},
    });
    queryClient.setQueryData(
      ["groups"],
      [
        {
          id: "6",
          name: "无动画分组",
          account_count: 0,
          scheduling_open: 0,
          scheduling_closed: 0,
          scheduling_unknown: 0,
          strategy: "balanced",
          strategy_source: "global_default",
          participation_status: "participating",
          participation_reason: null,
          status: "empty",
        } satisfies GroupStatus,
      ],
    );

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AccountCreationSettingsCard
          fallbackConcurrency={10}
          fallbackPriority={1}
          initialScope="6"
        />
      </QueryClientProvider>,
    );

    expect(markup).toMatch(
      /<button(?=[^>]*aria-label="收起 无动画分组 设置")(?=[^>]*data-press-animation="none")[^>]*>/,
    );
  });
});
