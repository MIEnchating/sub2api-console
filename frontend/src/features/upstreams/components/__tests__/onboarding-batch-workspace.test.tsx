import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  OnboardingBatchActionBar,
  OnboardingCandidateIdentity,
  OnboardingAccountType,
  OnboardingInferredPlatformStatus,
  OnboardingStepIndicator,
  OnboardingUpstreamSummary,
  onboardingSelectionLayout,
} from "../onboarding-batch-workspace";

describe("账号添加批量工作区", () => {
  it("把上游信息收拢为带可访问名称的紧凑概览", () => {
    const markup = renderToStaticMarkup(
      <OnboardingUpstreamSummary
        name="QYAI"
        baseUrl="https://ai.example.test"
        typeLabel="Sub2API"
        balance="49.99"
        rechargeRatio="1:1"
        selectableCount={11}
        boundCount={2}
        controls={<span data-slot="visibility-control">仅显示启用分组</span>}
      />,
    );

    expect(markup).toContain('aria-label="当前上游概况"');
    expect(markup).toContain("QYAI");
    expect(markup).toContain("Sub2API");
    expect(markup).toContain("49.99");
    expect(markup).toContain("11 个可选");
    expect(markup).toContain("2 个已绑定");
    expect(markup).toContain('data-slot="visibility-control"');
    expect(markup).toContain("仅显示启用分组");
    expect(markup).toContain("flex-wrap");
  });

  it("让批量参数和预览操作保持在可见的粘性操作栏中", () => {
    const markup = renderToStaticMarkup(
      <OnboardingBatchActionBar
        controls={<label htmlFor="batch-note">批量备注</label>}
        selectedCount={3}
        pending={false}
        disabled={false}
        onSubmit={() => undefined}
      />,
    );

    expect(markup).toContain('role="toolbar"');
    expect(markup).toContain('aria-label="批量添加账号"');
    expect(markup).toContain("sticky");
    expect(markup).toContain("bottom-0");
    expect(markup).toContain("3 项待提交");
    expect(markup).toContain("预览 3 项变更");
  });

  it("任务执行中禁用提交并显示明确状态", () => {
    const markup = renderToStaticMarkup(
      <OnboardingBatchActionBar
        controls={<span>批量参数</span>}
        selectedCount={1}
        pending
        disabled={false}
        onSubmit={() => undefined}
      />,
    );

    expect(markup).toContain('disabled=""');
    expect(markup).toContain("正在提交");
  });

  it("进入第二步后隐藏顶部步骤条", () => {
    const firstStep = renderToStaticMarkup(<OnboardingStepIndicator completed={false} />);
    const secondStep = renderToStaticMarkup(<OnboardingStepIndicator completed />);

    expect(firstStep).toContain("添加上游并完成鉴权");
    expect(firstStep).toContain("选择分组并添加账号");
    expect(secondStep).toBe("");
  });

  it("第二步只保留分组列表的内部纵向滚动", () => {
    const selection = onboardingSelectionLayout("full", true);

    expect(selection.fixedContent).toBe(true);
    expect(selection.cardClassName).toContain("h-full");
    expect(selection.tablePanelClassName).toContain("flex-col");
    expect(selection.tablePanelClassName).toContain("overflow-hidden");
    expect(selection.tablePanelClassName).toContain("rounded-lg border");
    expect(selection.tableContainerClassName).toContain("overflow-auto");
    expect(selection.tableContainerClassName).not.toContain("max-h");
    expect(selection.tableContainerClassName).not.toContain("rounded-lg");
    expect(selection.tableContainerClassName).not.toContain("border");

    const firstStep = onboardingSelectionLayout("full", false);
    expect(firstStep.fixedContent).toBe(false);
    expect(firstStep.tablePanelClassName).toContain("rounded-lg border");
    expect(firstStep.tableContainerClassName).toContain("overflow-y-hidden");
  });

  it("上游未返回平台时显示未知且不要求手工选择协议", () => {
    const markup = renderToStaticMarkup(
      <OnboardingCandidateIdentity
        groupName="Deepseek"
        platformLabel={null}
        description="腾讯合集"
        status={<span>未绑定</span>}
      />,
    );

    expect(markup).toContain("Deepseek");
    expect(markup).toContain("未知");
    expect(markup).toContain("腾讯合集");
    expect(markup).not.toContain("选择账号协议");
    expect(markup).not.toContain('role="combobox"');
  });

  it("选择本地分组后展示由分组平台反推的账号类型", () => {
    const markup = renderToStaticMarkup(
      <OnboardingInferredPlatformStatus required selectedCount={1} platformLabel="OpenAI" />,
    );

    expect(markup).toContain('role="status"');
    expect(markup).toContain("账号类型：");
    expect(markup).toContain(">OpenAI<");
    expect(markup).toContain("由本地分组确定");
  });

  it("账号类型作为独立列值显示且不附加解释文案", () => {
    const markup = renderToStaticMarkup(
      <OnboardingAccountType required selectedCount={1} platformLabel="OpenAI" />,
    );

    expect(markup).toContain("OpenAI");
    expect(markup).not.toContain("账号类型：");
    expect(markup).not.toContain("由本地分组确定");
  });

  it("复合分组无法唯一反推时显示明确原因", () => {
    const markup = renderToStaticMarkup(
      <OnboardingInferredPlatformStatus required selectedCount={1} platformLabel={null} />,
    );

    expect(markup).toContain("所选本地分组无法唯一确定账号类型");
  });
});
