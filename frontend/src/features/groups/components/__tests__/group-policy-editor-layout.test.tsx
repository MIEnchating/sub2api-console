import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { GroupPolicyOverrideUpdate } from "../../../../api";
import { GroupPolicyEditorFields, groupPolicyDialogLayout } from "../group-policy-editor-fields";

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
    expect(groupPolicyDialogLayout.body).toContain("overflow-x-hidden");
  });

  it("弹窗正文独立纵向滚动并保留固定页脚", () => {
    expect(groupPolicyDialogLayout.content).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(groupPolicyDialogLayout.content).toContain("overflow-hidden");
    expect(groupPolicyDialogLayout.body).toContain("min-h-0");
    expect(groupPolicyDialogLayout.body).toContain("overflow-y-auto");
    expect(groupPolicyDialogLayout.body).not.toContain("overflow-hidden");
  });
});
