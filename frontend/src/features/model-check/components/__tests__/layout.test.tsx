import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AccountStatus, Task } from "@/api";

import { ModelCheckPage } from "../model-check-page";
import { ModelCheckResult } from "../model-check-result";
import { ModelCheckSelection } from "../model-check-selection";

const account: AccountStatus = {
  id: "16",
  name: "用于验证超长账号名称不会挤压操作区域的测试账号",
  groups: ["default"],
  upstream_id: "up_test",
  upstream_host: "api.example.test",
  upstream_type: "oauth",
  platform: "openai",
  account_type: "oauth",
  schedulable: true,
  priority: 10,
  load_factor: "1",
  concurrency: 5,
  multiplier: "1",
  balance: null,
  paused: false,
  paused_reason: null,
  routing_state: "active",
  health_status: "健康",
  health: "healthy",
  desired_health: "healthy",
  apply_pending: false,
  apply_error: null,
  decision_state: "healthy",
  decision_reason: null,
  failure_streak: 0,
  recovery_pass_streak: 1,
  target_priority: 10,
  target_load_factor: "1",
  target_schedulable: true,
  target_concurrency: 5,
  health_score: 100,
  short_score: 100,
  long_score: 100,
  sample_count: 1,
  model_check_status: "consistent",
  model_check_checked_at: "2026-08-31T02:15:00Z",
  recent_results: [],
  ttfb_p50_ms: 250,
  ttfb_p95_ms: 400,
  weight: 100,
};

function selectionMarkup(accounts: AccountStatus[] = [account]): string {
  return renderToStaticMarkup(
    <ModelCheckSelection
      accounts={accounts}
      accountsLoading={false}
      accountsError={null}
      accountQuery=""
      selectedAccountIDs={["16"]}
      models={["gpt-5.6-sol"]}
      selectedModels={["gpt-5.6-sol"]}
      modelsLoading={false}
      modelsError={null}
      rounds={2}
      timeoutSeconds={45}
      combinationCount={1}
      selectionError={null}
      disabled={false}
      canSubmit
      onAccountQueryChange={() => undefined}
      onAccountToggle={() => undefined}
      onAccountsSelectAll={() => undefined}
      onClear={() => undefined}
      onModelToggle={() => undefined}
      onModelsSelectAll={() => undefined}
      onRefreshModels={() => undefined}
      onRoundsChange={() => undefined}
      onTimeoutChange={() => undefined}
      onSubmit={() => undefined}
    />,
  );
}

describe("模型检测响应式布局", () => {
  it("页面内容区域固定并由账号和模型面板占满剩余高度", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["accounts"], [account]);
    queryClient.setQueryData(["model-check-capabilities"], {
      claude_standards: [],
      sol_models: ["gpt-5.6-sol"],
    });
    queryClient.setQueryData(["model-check-account-statuses"], []);

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <ModelCheckPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("min-h-0 flex-1 overflow-hidden px-3");
    expect(markup).toContain("flex h-full min-h-0 flex-col");
    expect(markup).toContain("grid min-h-0 flex-1 items-stretch");
  });

  it("宽屏使用账号与矩阵双栏且窄屏保持纵向排列", () => {
    const markup = selectionMarkup();

    expect(markup).toContain("xl:grid-cols-[minmax(0,1fr)_22rem]");
    expect(markup.match(/h-\[32rem\] gap-0 py-0 xl:h-full/g)).toHaveLength(2);
    expect(markup).toContain('data-testid="model-check-account-mobile-list"');
    expect(markup).toContain('data-testid="model-check-account-desktop-table"');
    expect(markup).toContain("divide-y md:hidden");
    expect(markup).toContain("hidden min-h-0 flex-1 overflow-auto md:block");
    expect(markup).not.toContain("当前 1 个");
    expect(markup).not.toContain("健康");
    expect(markup).toContain("检测状态");
    expect(markup).toContain("上次检测");
    expect(markup).toContain("符合特征");
    expect(markup).toContain("08/31");
  });

  it("账号列表默认每页展示十条并显示筛选后的总数", () => {
    const accounts = Array.from({ length: 12 }, (_, index) => ({
      ...account,
      id: `${index + 1}`,
      name: `分页账号 ${index + 1}`,
    }));
    const markup = selectionMarkup(accounts);

    expect(markup).toContain("分页账号 10");
    expect(markup).not.toContain("分页账号 11");
    expect(markup).toContain(">12</span>");
    expect(markup).toContain('aria-label="转到第 2 页"');
  });

  it("检测状态只显示账号最近一次特征判定的三态结果", () => {
    const markup = selectionMarkup([
      { ...account, id: "16", model_check_status: "consistent" },
      { ...account, id: "17", model_check_status: "inconsistent" },
      { ...account, id: "18", model_check_status: "inconclusive" },
    ]);

    expect(markup).toContain("符合特征");
    expect(markup).toContain("不符合特征");
    expect(markup).toContain("无法判定");
    expect(markup).not.toContain("检测完成");
    expect(markup).not.toContain("检测失败");
  });

  it("人工优先位账号不可加入模型检测矩阵", () => {
    const markup = selectionMarkup([
      { ...account, manual_priority: 3, manual_sync_balance_multiplier: true },
    ]);

    expect(markup).toContain("人工控制");
    expect(markup).toContain(
      'aria-label="选择账号 用于验证超长账号名称不会挤压操作区域的测试账号"',
    );
    expect(markup).toContain("disabled");
  });

  it("轮次选择公开当前状态且汇总账号模型和组合数量", () => {
    const markup = selectionMarkup();

    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('data-slot="segmented-control"');
    expect(markup).toContain("账号");
    expect(markup).toContain("模型");
    expect(markup).toContain("组合");
    expect(markup).toContain("开始检测 · 1 个组合");
  });

  it("结果为桌面表格和移动明细提供独立布局且错误正文始终可见", () => {
    const task: Task = {
      id: "model-check-layout",
      skill: "sub2api-model-check",
      operation: "account-model-behavior-check",
      status: "succeeded",
      progress: 100,
      message: "检测完成",
      result: {
        tests: [
          {
            account_id: "16",
            account_name: account.name,
            claimed_model: "gpt-5.6-sol",
            verdict: "ERROR",
            error: "上游连接失败，错误详情在移动端也必须直接显示",
            requests: { successful: 0, total: 2 },
          },
        ],
      },
      created_at: "2026-08-28T00:00:00Z",
      updated_at: "2026-08-28T00:00:01Z",
    };

    const markup = renderToStaticMarkup(<ModelCheckResult task={task} />);

    expect(markup).toContain('data-testid="model-check-result-desktop-table"');
    expect(markup).toContain('data-testid="model-check-result-mobile-list"');
    expect(markup).toContain("hidden h-full overflow-auto md:block");
    expect(markup).toContain("h-full divide-y overflow-auto md:hidden");
    expect(markup).toContain("上游连接失败，错误详情在移动端也必须直接显示");
  });
});
