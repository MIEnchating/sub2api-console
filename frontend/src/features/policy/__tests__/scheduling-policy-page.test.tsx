import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  navItems,
  PolicyPage,
  PolicyRulesEditor,
  PolicyScopeEditor,
  PolicyScopeLayout,
  policyDraft,
  policyRelationshipError,
  schedulingStrategyOptions,
  withManagedGroupScope,
} from "../../../App";
import type { PolicySnapshot, RuntimeConfig } from "../../../api";

const policy: PolicySnapshot = {
  available: true,
  source: "console",
  mode: "完全模式",
  global_strategy: "balanced",
  group_strategies: [
    {
      id: "6",
      name: "codex",
      platforms: ["openai"],
      strategy: "price_first",
      strategy_source: "group_override",
      participation_status: "participating",
      participation_reason: null,
      account_count: 3,
    },
    {
      id: null,
      name: "缺少稳定 ID",
      platforms: [],
      strategy: "balanced",
      strategy_source: "global_default",
      participation_status: "configuration_error",
      participation_reason: "缺少稳定分组 ID",
      account_count: 1,
    },
  ],
  missing_rate_fallback: "current_cost_wall",
  change_threshold: "0.1",
  cooldown_seconds: 60,
  auto_apply: {
    schedulable: true,
    priority: true,
    load_factor: false,
    concurrency: true,
  },
  excluded_group_ids: [],
  traffic_enabled: true,
  probe_interval_seconds: 300,
  probe_model: "gpt-5.1-codex",
  traffic_lookback_minutes: 120,
  max_samples_per_account: 60,
  advanced_policy: {
    probe: { concurrency: 4 },
    traffic: { refresh_seconds: 60 },
    weights: { max_writes_per_group: 2, max_migration_ratio: 0.3 },
    upstream_multiplier: { interval_seconds: 120 },
    writeback: { concurrency: 4, verification: false },
    scaling: {
      enabled: false,
      global_max_concurrency: 900,
      min_per_account: 3,
      max_per_account: 250,
      scale_up_ratio: 0.8,
      step_up: 5,
      step_down: 5,
      cooldown_seconds: 60,
    },
  },
  configuration_errors: [],
};

const config: RuntimeConfig = {
  database_path: "/data/sub2api-console.sqlite3",
  data_database_path: "/data/sub2api-console.sqlite3",
  database_available: true,
  data_database_available: true,
  mode: "完全模式",
  config_keys: [],
  secret_values_hidden: true,
  probes_enabled: true,
  admin_base_url: "https://sub2api.example.test",
  request_timeout_seconds: 60,
  initialized: true,
  target_configured: true,
  console_username: "admin",
  configuration_errors: [],
};

function renderPolicyPage() {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["policy"], policy);
  queryClient.setQueryData(["config"], config);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <PolicyPage />
    </QueryClientProvider>,
  );
}

function policySection(markup: string, title: string, nextTitle?: string) {
  const start = markup.indexOf(`>${title}</div>`);
  const end = nextTitle
    ? markup.indexOf(`>${nextTitle}</div>`, start + title.length)
    : markup.length;
  expect(start).toBeGreaterThanOrEqual(0);
  expect(end).toBeGreaterThan(start);
  return markup.slice(start, end);
}

function expectSwitchLabelAssociation(markup: string, title: string) {
  const titlePosition = markup.indexOf(`>${title}</div>`);
  const labelStart = markup.lastIndexOf("<label", titlePosition);
  const labelEnd = markup.indexOf("</label>", titlePosition);
  const labelMarkup = markup.slice(labelStart, labelEnd);
  const controlId = labelMarkup.match(/for="([^"]+)"/)?.[1];

  expect(labelStart).toBeGreaterThanOrEqual(0);
  expect(labelEnd).toBeGreaterThan(titlePosition);
  expect(controlId).toBeTruthy();
  expect(markup).toContain(`id="${controlId}"`);
}

describe("调度策略入口", () => {
  it("在侧栏使用调度策略名称且保留原策略路由", () => {
    const item = navItems.find((candidate) => candidate.id === "policy");

    expect(item?.label).toBe("调度策略");
    expect(item?.to).toBe("/policy");
  });

  it("只展示全局调度规则并把分组策略留在分组管理", () => {
    const markup = renderPolicyPage();

    expect(markup).toContain("调度策略");
    expect(markup).not.toContain("策略中心");
    expect(markup).toContain("分组没有单独设置时使用");
    expect(markup).toContain("运营配置");
    expect(markup).toContain("系统级规则");
    expect(markup).toContain("守护范围");
    expect(markup).toContain("健康回池");
    expect(markup).toContain("负载因子调权");
    expect(markup).toContain("运行控制");
    expect(markup).toContain("随“保存策略”一起生效");
    expect(markup).toContain("执行模式");
    expect(markup).toContain("监控模式");
    expect(markup).toContain(
      "只读取上游与真实流量数据并执行巡检告警，不主动探测、不保存调度结果、不写入 Sub2API",
    );
    expect(markup).toContain("调度模式");
    expect(markup).toContain("采集上游与流量数据，计算并保存本地调度结果，不自动执行远程变更");
    expect(markup).toContain("完全模式");
    expect(markup).toContain('aria-label="启用主动探测"');
    expect(schedulingStrategyOptions).toContainEqual({
      value: "reliability",
      label: "稳定优先",
    });
    expect(markup).not.toContain("高级策略");
    expect(markup).not.toContain("JSON 对象");
    expect(markup).not.toContain("分组实际策略");
    expect(markup).not.toContain("data-policy-group-id");
    expect(markup).not.toContain("设置 codex 调度策略");
    expect(markup).toContain('aria-label="调度状态自动执行"');
    expect(markup).toContain('aria-label="负载因子自动执行"');
    expect(markup).not.toContain("观察模式自动执行");
    expect(policyDraft(policy).auto_apply).toEqual({
      schedulable: true,
      priority: true,
      load_factor: false,
      concurrency: true,
    });
    expect(policyDraft(policy).mode).toBe("完全模式");
    expect(navItems.find((candidate) => candidate.id === "groups")?.to).toBe("/groups");
  });

  it("使用与系统设置一致的紧凑布局且不再展示顶部策略指标", () => {
    const markup = renderPolicyPage();

    expect(markup).toContain('class="w-full space-y-4"');
    expect(markup.match(/data-slot="card"/g)).toHaveLength(11);
    expect(markup.match(/data-slot="card" data-card-hover="false" data-size="sm"/g)).toHaveLength(
      11,
    );
    expect(markup).not.toContain("探活来源");
    expect(markup).not.toContain("60s");
    expect(markup).not.toContain("xl:grid-cols-4");
    expect(markup).toContain('data-testid="policy-auto-apply"');
    expect(markup).toContain('data-testid="policy-runtime-modes"');
    expect(markup).toContain("flex-wrap");
    expect(markup.match(/data-slot="select-trigger"[^>]*class="[^"]*w-full/g)).toHaveLength(3);
    expect(markup).toContain('data-testid="policy-operations-layout"');
    expect(markup).toContain('class="flex flex-col gap-4"');
    expect(policySection(markup, "全局默认策略", "巡检任务周期")).toContain("lg:grid-cols-3");
    expect(markup).toContain("min-h-14 flex-col items-stretch justify-between");
    expect(markup).toContain("rounded-lg border px-3 py-2.5");
    expect(policySection(markup, "自动执行范围", "熔断")).toContain("xl:col-span-2");
    expect(policySection(markup, "熔断", "保底与降级")).toContain(
      "flex items-start justify-between gap-4",
    );
    expect(policySection(markup, "熔断", "保底与降级")).not.toContain(
      "flex-row items-start justify-between gap-4",
    );
    expectSwitchLabelAssociation(markup, "熔断");
    expectSwitchLabelAssociation(markup, "健康回池");
    expectSwitchLabelAssociation(markup, "智能扩容");
    expect(markup).toContain('for="policy-auto-apply-schedulable"');
    expect(markup).toContain('id="policy-auto-apply-schedulable"');
  });

  it("加载完整策略前显示稳定页面骨架", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(["config"], config);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <PolicyPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain('data-testid="policy-loading"');
    expect(markup).toContain('aria-label="正在加载调度策略"');
    expect(markup).not.toContain('data-testid="policy-runtime-modes"');
  });

  it("按参考项目归类全局策略、自动执行、调权和采样字段", () => {
    const markup = renderPolicyPage();
    const runtime = policySection(markup, "运行控制", "全局默认策略");
    const global = policySection(markup, "全局默认策略", "巡检任务周期");
    const inspectionIntervals = policySection(markup, "巡检任务周期", "自动执行范围");
    const autoApply = policySection(markup, "自动执行范围", "熔断");
    const weights = policySection(markup, "负载因子调权", "认证失效自动处置");
    const cleanup = policySection(markup, "认证失效自动处置", "智能扩容");
    const scaling = policySection(markup, "智能扩容", "采样（真实流量 / 主动探测）");
    const sampling = policySection(markup, "采样（真实流量 / 主动探测）");

    expect(runtime).toContain("执行模式");
    expect(runtime).not.toContain("启用主动探测");

    expect(global).toContain("倍率缺失回退");
    expect(global).toContain("每组权重预算");
    expect(global).toContain("权重健康闸门");
    expect(global).toContain("均衡中价格占比");
    expect(global).not.toContain("上游数据拉取间隔");
    expect(global).not.toContain("默认探测模型");
    expect(global).not.toContain("探测间隔");
    expect(global).not.toContain("流量回溯");
    expect(global).not.toContain("每账号样本上限");
    expect(global).not.toContain("调权冷却");
    expect(global).not.toContain("变化阈值");
    expect(global).not.toContain("自动执行");

    expect(inspectionIntervals).toContain("上游数据拉取间隔（秒）");
    expect(inspectionIntervals).toContain("请求记录拉取间隔（秒）");
    expect(inspectionIntervals).toContain("同时刷新倍率、余额和鉴权状态");
    expect(inspectionIntervals).toContain('min="30"');
    expect(inspectionIntervals).toContain('min="1"');
    expect(inspectionIntervals).toContain('value="120"');
    expect(inspectionIntervals).toContain('value="60"');

    expect(autoApply).toContain("调度状态");
    expect(autoApply).toContain("负载因子");
    expect(autoApply).toContain("远程写入并发");
    expect(autoApply).toContain('aria-label="写后确认"');
    expect(autoApply).not.toContain("每组每轮自动执行账号上限");
    expect(autoApply).not.toContain("每组每轮自动执行占比上限");
    expect(markup).toContain("窗口内慢响应次数");
    expect(markup).toContain("每轮最多熔断");
    expect(markup).toContain("保底与降级");
    expect(markup).toContain("认证失效自动处置");
    expect(markup.indexOf("认证失效自动处置")).toBeLessThan(markup.indexOf("智能扩容"));
    expect(weights).toContain("变化阈值");
    expect(weights).toContain("调权冷却（秒）");
    expect(weights).toContain("价格权重强度");
    expect(weights).toContain("速度权重强度");
    expect(weights.match(/min="0.000001"/g)).toHaveLength(3);
    expect(weights.match(/max="100"/g)).toHaveLength(2);
    expect(weights).not.toContain("微调防抖阈值");

    expect(scaling).toContain("启用智能扩容");
    expect(scaling).toContain("全局并发上限");
    expect(scaling).toContain("单账号并发下限");
    expect(scaling).toContain("单账号并发上限");
    expect(sampling).toContain("探测并发");
    expect(sampling).not.toContain("真实流量并发");
    expect(sampling).toContain('value="4"');
    expect(scaling).toContain("扩容触发负载率");
    expect(scaling).toContain("扩容步长");
    expect(scaling).toContain("缩容步长");
    expect(scaling).toContain("扩缩容冷却（秒）");
    expect(scaling).toContain('value="900"');
    expect(scaling).toContain('value="3"');
    expect(scaling).toContain('value="250"');
    expect(scaling).toContain('value="0.8"');
    expect(scaling.match(/value="5"/g)).toHaveLength(2);
    expect(scaling).toContain('value="60"');
    expect(cleanup).toContain("处置动作");

    expect(sampling).not.toContain("健康来源");
    expect(sampling).toContain("接入真实流量样本");
    expect(sampling).toContain("无新鲜流量的账号回退主动探测");
    expect(sampling).toContain("启用主动探测");
    expect(sampling).toContain("默认探测模型");
    expect(sampling).toContain("探测间隔（秒）");
    expect(sampling).toContain('min="30"');
    expect(sampling).not.toContain("流量证据拉取间隔");
    expect(sampling).toContain("流量回溯（分钟）");
    expect(sampling).toContain("每账号样本上限");
    expect(sampling).toContain("探测超时（秒）");
    expect(sampling).not.toContain("单次探测尝试次数（次）");
    expect(sampling).toContain("探测并发");
    expect(sampling).toContain('max="86400"');
    expect(sampling).not.toContain('max="3"');
    expect(sampling).toContain('max="32"');
    expect(sampling).toContain("真实样本新鲜期（秒）");
    expect(sampling).toContain("lg:grid-cols-3");
    expect(sampling).toContain('data-testid="policy-sampling-switches"');
    expect(sampling).toContain("lg:grid-cols-3");
    const switches = sampling.slice(sampling.indexOf('data-testid="policy-sampling-switches"'));
    expect(switches).toContain("接入真实流量样本");
    expect(switches).toContain("启用主动探测");
    expect(switches).toContain("有新鲜流量时跳过探测");
  });

  it("只接受真实流量优先或纯主动探测两种模式", () => {
    expect(policyDraft({ ...policy, traffic_enabled: true }).traffic_enabled).toBe(true);
    expect(policyDraft({ ...policy, traffic_enabled: false }).traffic_enabled).toBe(false);
  });

  it("在前端阻止有冲突的策略字段关系", () => {
    const value = policyDraft({
      ...policy,
      advanced_policy: {
        scoring: {
          short_window: 10,
          long_window: 5,
          event_scores: { quota_exhausted: 15 },
        },
      },
    });
    expect(policyRelationshipError(value)).toBe("长期窗口不能小于短期窗口");

    value.advanced_policy.scoring = {
      short_window: 10,
      long_window: 60,
      event_scores: { quota_exhausted: 0 },
    };
    expect(policyRelationshipError(value)).toBe("限流 / 额度耗尽分必须大于 0");
  });

  it("系统规则只展示参考项目的健康公式和错误分类字段", () => {
    const value = policyDraft({
      ...policy,
      advanced_policy: {
        scoring: {
          short_window: 10,
          long_window: 60,
          latest_weight: 0.5,
          short_ratio: 0.7,
          slow_ttfb_ms: 5000,
          event_scores: { fatal: 0 },
        },
        classify: {
          fatal_patterns: ["unauthorized"],
          gateway_status_codes: [429, 500, 502, 503, 504],
        },
      },
    });
    const markup = renderToStaticMarkup(
      <PolicyRulesEditor value={value} onChange={() => undefined} />,
    );

    expect(markup).toContain("当前公式");
    expect(markup).toContain("最终分 = 短期分 × 0.70 + 长期分 × 0.30");
    expect(markup).toContain("致命错误关键字");
    expect(markup).toContain("网关错误状态码");
    expect(markup).not.toContain("网关错误关键字");
    expect(markup).not.toContain("限流 / 额度关键字");
    expect(markup).not.toContain("几何衰减");
    expect(markup).not.toContain("无样本健康分");
  });

  it("守护范围仅用稳定账号 ID 并提供交还控制权", () => {
    const markup = renderToStaticMarkup(
      <PolicyScopeEditor
        value={policyDraft(policy)}
        onChange={() => undefined}
        groups={policy.group_strategies}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain("暂停与排除的渠道");
    expect(markup).toContain("暂停调度的稳定账号 ID");
    expect(markup).toContain("排除的稳定账号 ID");
    expect(markup).toContain("交还控制权");
    expect(markup).not.toContain("暂停的分组");
    const participating = policySection(markup, "参与守护的分组", "账号类型与平台");
    expect(participating).toContain("全部分组");
    expect(participating).toContain("当前所有分组都参与守护");
    expect(participating).not.toContain("分组名称");
  });

  it("关闭全部分组后按稳定 ID 展示并保存可选分组", () => {
    const selected = withManagedGroupScope(policyDraft(policy), "selected", ["6"]);
    const markup = renderToStaticMarkup(
      <PolicyScopeEditor
        value={selected}
        onChange={() => undefined}
        groups={policy.group_strategies}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain('data-testid="managed-group-options"');
    expect(markup).toContain("codex");
    expect(markup).toContain("#6 · openai");
    expect(markup).toContain('aria-label="选择分组 codex"');
    expect(selected.advanced_policy.scope).toMatchObject({
      managed_group_mode: "selected",
      managed_group_ids: ["6"],
    });
  });

  it("守护范围按全宽单列排列卡片", () => {
    const markup = renderToStaticMarkup(
      <PolicyScopeLayout
        value={policyDraft(policy)}
        onChange={() => undefined}
        groups={policy.group_strategies}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain('data-testid="policy-scope-layout"');
    expect(markup).toContain('class="flex flex-col gap-4"');
    expect(markup).not.toContain("xl:grid-cols-2");
  });
});
