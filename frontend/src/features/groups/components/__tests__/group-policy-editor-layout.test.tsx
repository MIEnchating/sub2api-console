import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { GroupPolicyOverrideUpdate } from "../../../../api";
import {
  GroupPolicyEditorFields,
  groupPolicyDialogLayout,
  groupProbeModelOptions,
} from "../group-policy-editor-fields";

const value: GroupPolicyOverrideUpdate = {
  enabled: true,
  strategy: "balanced",
  min_pool_size: 1,
  weight_budget: 400,
  balanced_price_ratio: 0.5,
  breaker_enabled: true,
  recovery_enabled: true,
  weights_enabled: true,
  scaling_enabled: false,
  probe_enabled: true,
  probe_interval_seconds: 300,
  probe_model: "claude-sonnet-4-6",
};

function section(markup: string, start: string, end?: string): string {
  const startIndex = markup.indexOf(start);
  const endIndex = end ? markup.indexOf(end, startIndex + start.length) : markup.length;
  return markup.slice(startIndex, endIndex);
}

describe("分组策略编辑布局", () => {
  it("自动获取模型后去重排序并保留当前分组模型", () => {
    expect(
      groupProbeModelOptions(["gpt-5.2", "gpt-5.1-codex", "gpt-5.2", ""], "custom-probe-model"),
    ).toEqual(["custom-probe-model", "gpt-5.1-codex", "gpt-5.2"]);

    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields
        value={value}
        onChange={() => undefined}
        onReloadProbeModels={() => undefined}
        probeModels={{
          group_id: "6",
          group_name: "codex",
          models: ["gpt-5.1-codex", "gpt-5.2"],
          account_count: 2,
          accounts_with_models: 2,
          complete: true,
        }}
      />,
    );
    const probe = section(markup, 'data-testid="group-policy-probe-settings"');

    expect(probe).toContain('aria-label="选择测试模型"');
    expect(probe).toContain("重新获取组内模型");
    expect(probe).toContain('type="button"');
    expect(probe).not.toContain('disabled=""');
    expect(probe).not.toContain("已自动获取");
    expect(probe).not.toContain("覆盖 2 / 2 个账号");
  });

  it("初次自动获取模型时显示加载骨架而不展示模型输入框", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields
        value={value}
        onChange={() => undefined}
        probeModelsLoading
        onReloadProbeModels={() => undefined}
      />,
    );
    const probe = section(markup, 'data-testid="group-policy-probe-settings"');

    expect(probe).toContain('aria-label="正在自动获取组内模型"');
    expect(probe).toContain("animate-pulse");
    expect(probe).toContain("正在获取");
    expect(probe).not.toContain('value="claude-sonnet-4-6"');
  });

  it("调度策略使用四个等尺寸选项且选中态不改变尺寸", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields value={value} onChange={() => undefined} />,
    );
    const strategies = section(markup, "<fieldset", "保底可用账号数");

    expect(strategies).toContain("sm:grid-cols-4");
    expect(strategies.match(/h-9 w-full min-w-0/g)).toHaveLength(4);
    expect(strategies.match(/role="radio"/g)).toHaveLength(4);
    expect(strategies).toContain('aria-checked="true"');
    expect(strategies).toContain('aria-checked="false"');
  });

  it("展示所选策略的实际计算公式", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields
        value={{ ...value, strategy: "speed_first" }}
        onChange={() => undefined}
      />,
    );

    expect(markup).toContain("80% 相对速度 + 20% 相对价格");
    expect(markup).toContain("最终权重 = 组内预算 × 质量分 ÷ 质量分总和");
  });

  it("定时测试独立于四项策略能力并与测试参数放在同一区域", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields value={value} onChange={() => undefined} />,
    );
    const capabilities = section(
      markup,
      'data-testid="group-policy-capability-switches"',
      'data-testid="group-policy-probe-settings"',
    );
    const probe = section(markup, 'data-testid="group-policy-probe-settings"');

    expect(capabilities).toContain("熔断");
    expect(capabilities).toContain("健康回池");
    expect(capabilities).toContain("负载因子调权");
    expect(capabilities).toContain("智能扩容");
    expect(capabilities).toContain("连续失败达到条件后触发熔断");
    expect(capabilities).not.toContain("定时测试");
    expect(probe).toContain("定时测试");
    expect(probe).toContain("测试间隔（秒）");
    expect(probe).toContain("测试模型");
  });

  it("字段限制最小宽度且弹窗正文隐藏横向溢出", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields value={value} onChange={() => undefined} />,
    );

    expect(markup).toContain("min-w-0");
    expect(markup).toContain('aria-label="组内总权重预算说明"');
    expect(markup).not.toContain("由同组参与调度的账号按策略共享");
    expect(groupPolicyDialogLayout.body).toContain("overflow-x-hidden");
  });

  it("数字策略字段清空后保持空白", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields
        value={{
          ...value,
          min_pool_size: null,
          weight_budget: null,
          balanced_price_ratio: null,
          probe_interval_seconds: null,
        }}
        onChange={() => undefined}
      />,
    );

    expect(markup).toContain('value=""');
    expect(markup).not.toContain('value="0"');
  });

  it("弹窗正文独立纵向滚动并保留固定页脚", () => {
    expect(groupPolicyDialogLayout.content).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(groupPolicyDialogLayout.content).toContain("overflow-hidden");
    expect(groupPolicyDialogLayout.body).toContain("min-h-0");
    expect(groupPolicyDialogLayout.body).toContain("overflow-y-auto");
    expect(groupPolicyDialogLayout.body).not.toContain("overflow-hidden");
  });
});
