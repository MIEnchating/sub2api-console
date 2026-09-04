import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  navItems,
  PolicyInspectionSchedule,
  PolicyOperationsEditor,
  PolicyPage,
  PolicyRulesEditor,
  PolicyScopeEditor,
  PolicyScopeLayout,
  policyPayload,
  policyDraft,
  policySettingsPayload,
  policyRelationshipError,
  schedulingStrategyOptions,
  withManagedGroupScope,
} from "../../../App";
import type { AccountStatus, PolicySnapshot, RuntimeConfig } from "../../../api";

const accounts = [
  {
    id: "41",
    name: "星筱主账号",
    groups: ["codex"],
    platform: "openai",
    account_type: "oauth",
    upstream_type: "newapi",
  },
] as AccountStatus[];

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
    probe: {
      concurrency: 4,
      retry_enabled: true,
      retry_source: "fixed",
      retry_count: 2,
      retry_status_codes: [429, 503],
    },
    traffic: { refresh_seconds: 60 },
    weights: {},
    manual_priority: { reserved_max: 10 },
    scope: {
      manage_all_accounts: true,
      managed_group_mode: "all",
      managed_group_ids: [],
    },
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
  account_default_concurrency: 10,
  account_default_priority: 1,
  admin_base_url: "https://sub2api.example.test",
  request_timeout_seconds: 60,
  initialized: true,
  target_configured: true,
  console_username: "admin",
  configuration_errors: [],
};

function renderPolicyPage(snapshot: PolicySnapshot = policy) {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["policy"], snapshot);
  queryClient.setQueryData(["config"], config);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <PolicyPage />
    </QueryClientProvider>,
  );
}

function renderOperationsSection(
  section: "routing" | "health" | "sampling",
  snapshot: PolicySnapshot = policy,
) {
  return renderToStaticMarkup(
    <PolicyOperationsEditor
      section={section}
      value={policyDraft(snapshot)}
      onChange={() => undefined}
      probesEnabled
      probesPending={false}
      onProbesEnabledChange={() => undefined}
    />,
  );
}

function policySection(markup: string, title: string, nextTitle?: string) {
  const start = markup.indexOf(`>${title}</div>`);
  const nextTitlePosition = nextTitle
    ? markup.indexOf(`>${nextTitle}</div>`, start + title.length)
    : -1;
  const nextCardPosition =
    nextTitlePosition > start ? markup.lastIndexOf('<div data-slot="card"', nextTitlePosition) : -1;
  const end =
    nextCardPosition > start
      ? nextCardPosition
      : nextTitlePosition > start
        ? nextTitlePosition
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
    expect(markup).toContain("价格和速度先在组内换算为 0～1 的相对分数");
    expect(markup).toContain("最终权重 = 组内预算 × 质量分 ÷ 质量分总和");
    expect(markup).toContain("调度与写入");
    expect(markup).toContain("健康与处置");
    expect(markup).toContain("巡检与采样");
    expect(markup).toContain("守护范围");
    expect(markup).not.toContain("目标盈利比例");
    expect(markup).not.toContain("余额告警阈值");
    expect(markup).not.toContain("Client Secret");
    expect(markup).not.toContain("系统级规则");
    expect(markup).toContain("负载因子调权");
    expect(markup).not.toContain("运行控制");
    expect(markup).not.toContain("执行模式");
    expect(markup).not.toContain("监控模式");
    expect(markup).not.toContain("调度模式");
    expect(renderOperationsSection("sampling")).toContain('aria-label="启用主动探测"');
    expect(schedulingStrategyOptions).toContainEqual({
      value: "reliability",
      label: "稳定优先",
      description: "质量分 = 健康门控 ×（75% 健康稳定性 + 15% 相对速度 + 10% 相对价格）",
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
    expect(policySettingsPayload(policyPayload(policyDraft(policy))!)).not.toHaveProperty("mode");
    expect(navItems.find((candidate) => candidate.id === "groups")?.to).toBe("/groups");
  });

  it("旧策略缺少倍率同步分区时使用兼容默认值并允许保存", () => {
    const legacy = {
      ...policy,
      advanced_policy: { ...policy.advanced_policy },
    };
    delete legacy.advanced_policy.account_rate_sync;

    const markup = renderPolicyPage(legacy);
    const schedule = renderToStaticMarkup(
      <PolicyInspectionSchedule value={policyDraft(legacy)} onChange={() => undefined} />,
    );

    expect(markup).not.toContain("策略参数存在空值或无效数字");
    expect(schedule).toContain("倍率同步每轮数量");
    expect(schedule).toContain("倍率同步每轮比例");
  });

  it("清空倍率同步间隔时保留空值并由保存校验拦截", () => {
    const clearedSnapshot = {
      ...policy,
      advanced_policy: {
        ...policy.advanced_policy,
        account_rate_sync: { interval_seconds: null, batch_size: null, batch_percent: null },
      },
    } satisfies PolicySnapshot;
    const cleared = policyDraft(clearedSnapshot);
    const markup = renderToStaticMarkup(
      <PolicyInspectionSchedule value={cleared} onChange={() => undefined} />,
    );
    const pageMarkup = renderPolicyPage(clearedSnapshot);
    const saveLabelPosition = pageMarkup.indexOf("保存策略");
    const saveButtonStart = pageMarkup.lastIndexOf("<button", saveLabelPosition);
    const saveButtonEnd = pageMarkup.indexOf(">", saveButtonStart);
    const labelPosition = markup.indexOf("账号倍率同步间隔（秒）");
    const inputStart = markup.indexOf("<input", labelPosition);
    const inputEnd = markup.indexOf(">", inputStart);

    expect(markup.slice(inputStart, inputEnd)).not.toContain('value="120"');
    expect(markup).not.toContain('value="0"');
    expect(policyPayload(cleared)).toBeNull();
    expect(pageMarkup).not.toContain("策略参数存在空值或无效数字");
    expect(pageMarkup.slice(saveButtonStart, saveButtonEnd)).not.toMatch(/\sdisabled(?:=|>)/);
  });

  it("使用可持续定位的页签和按业务成组的紧凑布局", () => {
    const markup = renderPolicyPage();

    expect(markup).toContain('data-testid="policy-page-layout"');
    expect(markup).toContain('class="w-full space-y-3"');
    expect(markup).toContain('data-testid="policy-category-navigation"');
    expect(markup).toContain("sticky top-0");
    expect(markup).toContain("overflow-x-auto");
    expect(markup.match(/data-slot="card"/g)).toHaveLength(4);
    expect(markup.match(/data-slot="card" data-card-hover="false" data-size="sm"/g)).toHaveLength(
      4,
    );
    expect(markup).not.toContain("探活来源");
    expect(markup).not.toContain("60s");
    expect(markup).not.toContain("xl:grid-cols-4");
    expect(markup).toContain('data-testid="policy-auto-apply"');
    expect(markup).not.toContain('data-testid="policy-runtime-modes"');
    expect(markup).toContain('data-slot="segmented-control"');
    expect(markup).toContain("flex-wrap");
    expect(markup.match(/data-slot="select-trigger"[^>]*class="[^"]*w-full/g)).toHaveLength(2);
    expect(markup).toContain('data-testid="policy-operations-layout"');
    expect(markup).toContain('class="flex flex-col gap-3"');
    expect(markup).toContain('data-testid="policy-routing-overview"');
    expect(markup).toContain('data-testid="policy-routing-sections"');
    expect(markup).toContain("xl:grid-cols-2");
    expect(policySection(markup, "全局默认策略", "自动执行范围")).toContain("lg:grid-cols-3");
    expect(markup).toContain("min-h-14 flex-col items-stretch justify-between");
    expect(markup).toContain("rounded-lg border px-3 py-2.5");
    expect(markup).toMatch(/class="[^"]*xl:col-span-2[^"]*" data-policy-section="自动执行范围"/);
    const health = renderOperationsSection("health");
    expect(health).toContain("flex items-start justify-between gap-4");
    expect(health).not.toContain("flex-row items-start justify-between gap-4");
    expectSwitchLabelAssociation(health, "熔断");
    expectSwitchLabelAssociation(health, "健康回池");
    expectSwitchLabelAssociation(markup, "智能扩容");
    expect(markup).toContain('for="policy-auto-apply-schedulable"');
    expect(markup).toContain('id="policy-auto-apply-schedulable"');
  });

  it("四类策略编辑器在宽屏分栏时保持同排卡片等高", () => {
    const routing = renderOperationsSection("routing");
    const health = renderOperationsSection("health");
    const sampling = renderOperationsSection("sampling");
    const rules = renderToStaticMarkup(
      <PolicyRulesEditor value={policyDraft(policy)} onChange={() => undefined} />,
    );
    const scope = renderToStaticMarkup(
      <PolicyScopeLayout
        value={policyDraft(policy)}
        onChange={() => undefined}
        groups={policy.group_strategies}
        accounts={accounts}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(routing).toContain('data-testid="policy-routing-sections"');
    expect(health).toContain('data-testid="policy-health-sections"');
    expect(sampling).toContain('data-testid="policy-sampling-sections"');
    expect(rules).toContain('data-testid="policy-rules-sections"');
    expect(scope).toContain('data-testid="policy-scope-layout"');
    for (const section of [routing, health, sampling, rules, scope]) {
      expect(section).toContain("grid items-stretch gap-3 xl:grid-cols-2");
    }
    expect(health).toContain('data-policy-section="保底与降级"');
    expect(health).toContain('data-policy-section="健康回池"');
    expect(scope).toContain('data-policy-section="暂停与排除的账号"');
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
    const routing = renderOperationsSection("routing");
    const health = renderOperationsSection("health");
    const sampling = renderOperationsSection("sampling");
    const inspectionIntervals = renderToStaticMarkup(
      <PolicyInspectionSchedule value={policyDraft(policy)} onChange={() => undefined} />,
    );
    const global = policySection(markup, "全局默认策略", "自动执行范围");
    const autoApply = policySection(routing, "自动执行范围", "负载因子调权");
    const weights = policySection(routing, "负载因子调权", "智能扩容");
    const cleanup = policySection(health, "认证失效自动处置");
    const scaling = policySection(routing, "智能扩容");

    expect(global).toContain("倍率缺失回退");
    expect(global).toContain("每组总权重预算");
    expect(global).toContain("人工优先位范围");
    expect(global).toContain('aria-label="人工优先位范围说明"');
    expect(global).toContain("权重健康闸门");
    expect(global).toContain("均衡中价格占比");
    expect(global).toContain("性能最小样本数");
    expect(global).toContain("速度优势上限");
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
    expect(inspectionIntervals).toContain('aria-label="上游数据拉取间隔（秒）说明"');
    expect(inspectionIntervals).toContain('min="30"');
    expect(inspectionIntervals).toContain('min="1"');
    expect(inspectionIntervals).toContain('value="120"');
    expect(inspectionIntervals).toContain('value="60"');

    expect(autoApply).toContain("调度状态");
    expect(autoApply).toContain("负载因子");
    expect(autoApply).toContain("调度写入并发");
    expect(autoApply).toContain('aria-label="调度写后确认"');
    expect(autoApply).toContain("只复核自动调度实际修改的字段");
    expect(autoApply).not.toContain("Key 创建");
    expect(autoApply).not.toContain("账号添加");
    expect(autoApply).not.toContain("删除结果");
    expect(autoApply).not.toContain("每组每轮自动执行账号上限");
    expect(autoApply).not.toContain("每组每轮自动执行占比上限");
    expect(health).toContain("窗口内慢响应次数");
    expect(health).toContain("每轮最多熔断");
    expect(health).toContain("连续瞬时失败阈值");
    expect(health).toContain("保底与降级");
    expect(health).toContain("认证失效自动处置");
    expect(routing).not.toContain("认证失效自动处置");
    expect(health).not.toContain("智能扩容");
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
    expect(sampling).toContain("失败重试");
    expect(sampling).toContain('aria-label="探测失败重试模式"');
    expect(sampling).toContain('data-slot="segmented-control"');
    expect(sampling).toContain("固定规则");
    expect(sampling).toContain("跟随账号池");
    expect(sampling).toContain("失败重试次数");
    expect(sampling).toContain("触发重试状态码");
    expect(sampling).toContain('value="2"');
    expect(sampling).toContain('value="429, 503"');
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

  it("关闭重试时隐藏模式，跟随账号池时隐藏固定参数", () => {
    const disabledSampling = renderOperationsSection("sampling", {
      ...policy,
      advanced_policy: {
        ...policy.advanced_policy,
        probe: { retry_enabled: false, retry_source: "fixed", retry_count: 2 },
      },
    });
    expect(disabledSampling).toContain("失败重试");
    expect(disabledSampling).not.toContain("固定规则");
    expect(disabledSampling).not.toContain("失败重试次数");

    const poolSampling = renderOperationsSection("sampling", {
      ...policy,
      advanced_policy: {
        ...policy.advanced_policy,
        probe: { retry_enabled: true, retry_source: "sub2api_pool" },
      },
    });
    expect(poolSampling).toContain("固定规则");
    expect(poolSampling).toContain("跟随账号池");
    expect(poolSampling).toContain("读取每个 Sub2API 账号的池模式、重试次数和状态码");
    expect(poolSampling).not.toContain("失败重试次数");
    expect(poolSampling).not.toContain("触发重试状态码");
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
    expect(markup).toContain("客户端错误状态码");
    expect(markup).toContain("普通 403");
    expect(markup).not.toContain("网关错误关键字");
    expect(markup).not.toContain("限流 / 额度关键字");
    expect(markup).not.toContain("几何衰减");
    expect(markup).not.toContain("无样本健康分");
  });

  it("守护范围使用可搜索选项并提供交还控制权", () => {
    const markup = renderToStaticMarkup(
      <PolicyScopeEditor
        value={policyDraft(policy)}
        onChange={() => undefined}
        groups={policy.group_strategies}
        accounts={accounts}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain("暂停与排除的账号");
    expect(markup).toContain("选择账号类型");
    expect(markup).toContain("选择平台");
    expect(markup).toContain("选择排除分组");
    expect(markup).toContain("选择暂停账号");
    expect(markup).toContain("选择排除账号");
    expect(markup).toContain("交还控制权");
    expect(markup).not.toContain("例如 apikey, oauth");
    expect(markup).not.toContain("例如 12, 34");
    expect(markup).not.toContain("稳定账号 ID");
    expect(markup).not.toContain("暂停的分组");
    const participating = policySection(markup, "参与守护的分组", "账号类型与平台");
    expect(participating).toContain("全部分组");
    expect(participating).toContain("当前所有分组都参与守护");
    expect(participating).not.toContain("分组名称");
  });

  it("守护范围用可读名称展示已保存的类型、平台、分组和账号", () => {
    const configured = policyDraft({
      ...policy,
      excluded_group_ids: ["6"],
      advanced_policy: {
        ...policy.advanced_policy,
        scope: {
          ...(policy.advanced_policy.scope as Record<string, unknown>),
          account_types: ["oauth"],
          platforms: ["openai"],
          paused_account_ids: ["41"],
          excluded_account_ids: ["404"],
        },
      },
    });
    const markup = renderToStaticMarkup(
      <PolicyScopeEditor
        value={configured}
        onChange={() => undefined}
        groups={policy.group_strategies}
        accounts={accounts}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain("OAuth");
    expect(markup).toContain("openai");
    expect(markup).toContain("codex（#6）");
    expect(markup).toContain("星筱主账号（#41） · openai · codex");
    expect(markup).toContain("账号 #404（当前配置，列表中未找到）");
    expect(configured.excluded_group_ids).toEqual(["6"]);
    expect(configured.advanced_policy.scope).toMatchObject({
      paused_account_ids: ["41"],
      excluded_account_ids: ["404"],
    });
  });

  it("关闭全部分组后按稳定 ID 展示并保存可选分组", () => {
    const selected = withManagedGroupScope(policyDraft(policy), "selected", ["6"]);
    const markup = renderToStaticMarkup(
      <PolicyScopeEditor
        value={selected}
        onChange={() => undefined}
        groups={policy.group_strategies}
        accounts={accounts}
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

  it("账号托管默认开启并说明人工优先级例外", () => {
    const markup = renderToStaticMarkup(
      <PolicyScopeEditor
        value={policyDraft(policy)}
        onChange={() => undefined}
        groups={policy.group_strategies}
        accounts={accounts}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain("账号托管");
    expect(markup).toContain('aria-label="托管所有账号"');
    expect(markup).toContain('aria-checked="true"');
    expect(markup).toContain("人工优先级账号始终由人工控制");
    expect(policyDraft(policy).advanced_policy.scope).toMatchObject({
      manage_all_accounts: true,
    });
  });

  it("守护范围在宽屏保持同排卡片等高并让账号操作占满整行", () => {
    const markup = renderToStaticMarkup(
      <PolicyScopeLayout
        value={policyDraft(policy)}
        onChange={() => undefined}
        groups={policy.group_strategies}
        accounts={accounts}
        onRestoreControl={() => undefined}
        restorePending={false}
      />,
    );

    expect(markup).toContain('data-testid="policy-scope-layout"');
    expect(markup).toContain('class="grid items-stretch gap-3 xl:grid-cols-2"');
    expect(markup).toContain('data-policy-section="暂停与排除的账号"');
    expect(markup.match(/xl:col-span-2/g)?.length ?? 0).toBeGreaterThanOrEqual(2);
  });
});
