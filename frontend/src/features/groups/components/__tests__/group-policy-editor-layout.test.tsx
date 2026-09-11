import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { GroupPolicyOverrideUpdate } from "../../../../api";
import {
  GroupPolicyEditorFields,
  groupPolicyDialogLayout,
  groupProbeModelDraftValue,
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
  it("分组没有单独模型时保留继承状态而不固化全局模型", () => {
    expect(groupProbeModelDraftValue(undefined)).toBeNull();
    expect(groupProbeModelDraftValue(null)).toBeNull();
    expect(groupProbeModelDraftValue(" group-model ")).toBe("group-model");
  });

  it("自动获取模型后为手动输入框提供去重排序的组选项", () => {
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

    expect(probe).toContain('aria-label="手动输入探活模型"');
    expect(probe).toContain('aria-label="探活模型输入方式"');
    expect(probe).toContain("选择模型");
    expect(probe).not.toContain("<datalist");
    expect(probe).toContain("重新获取组内模型");
    expect(probe).not.toContain('disabled=""');
    expect(probe).not.toContain("已自动获取");
    expect(probe).not.toContain("覆盖 2 / 2 个账号");
  });

  it("初次自动获取模型时保留手动输入框并显示获取状态", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields
        value={value}
        onChange={() => undefined}
        probeModelsLoading
        onReloadProbeModels={() => undefined}
      />,
    );
    const probe = section(markup, 'data-testid="group-policy-probe-settings"');

    expect(probe).toContain('aria-label="手动输入探活模型"');
    expect(probe).toContain("正在获取");
    expect(probe).toContain('value="claude-sonnet-4-6"');
    expect(probe).not.toContain("<datalist");
  });

  it("调度策略包含全局默认且五个选项尺寸一致", () => {
    render(<GroupPolicyEditorFields value={value} onChange={() => undefined} />);
    const strategies = screen.getAllByRole("radio");
    expect(strategies).toHaveLength(5);
    for (const strategy of strategies) {
      expect(strategy).toHaveClass("h-8", "w-full", "min-w-0");
    }
    expect(screen.getByRole("radiogroup", { name: "调度策略" })).toHaveClass("sm:grid-cols-5");
    expect(screen.getByRole("radio", { name: "均衡" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "全局默认" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  it("探活模型输入方式切换按钮与策略选项保持相同高度", () => {
    render(<GroupPolicyEditorFields value={value} onChange={() => undefined} />);

    for (const name of ["手动输入", "选择模型"]) {
      expect(screen.getByRole("button", { name })).toHaveClass("h-8", "text-sm");
    }
    expect(screen.getByRole("button", { name: "手动输入" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
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

    expect(capabilities).toContain("自动熔断");
    expect(capabilities).toContain("健康回池");
    expect(capabilities).toContain("负载因子调权");
    expect(capabilities).toContain("智能扩容");
    for (const label of ["自动熔断", "健康回池", "负载因子调权", "智能扩容"]) {
      expect(capabilities).toContain(`aria-label="${label}说明"`);
    }
    expect(capabilities).not.toContain("故障达到条件后自动停止调度；开启健康回池后可自动恢复");
    expect(capabilities).not.toContain("定时测试");
    expect(probe).toContain("定时测试");
    expect(probe).toContain('aria-label="定时测试说明"');
    expect(probe).not.toContain("定期测试该分组账号，测试参数仅覆盖当前分组。");
    expect(probe).toContain("测试间隔（秒）");
    expect(probe).toContain("探活模型");
  });

  it("字段限制最小宽度且弹窗正文隐藏横向溢出", () => {
    const markup = renderToStaticMarkup(
      <GroupPolicyEditorFields value={value} onChange={() => undefined} />,
    );

    expect(markup).toContain("min-w-0");
    expect(markup).toContain('aria-label="参与守护说明"');
    expect(markup).toContain('aria-label="保底可用账号数说明"');
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
