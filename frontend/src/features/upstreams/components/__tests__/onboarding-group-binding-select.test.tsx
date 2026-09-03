import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  OnboardingGroupBindingOption,
  OnboardingGroupBindingSelect,
} from "../onboarding-group-binding-select";

describe("OnboardingGroupBindingSelect", () => {
  it("puts the local-group binding control directly in an upstream-group row", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingSelect
        upstreamGroupName="kiro-power"
        upstreamPlatform="anthropic"
        groups={[{ id: "8", name: "A-kiro逆向", platform: "anthropic" }]}
        value={[]}
        disabled={false}
        disabledReason={null}
        onValueChange={() => undefined}
      />,
    );

    expect(markup).toContain('aria-label="kiro-power 本地分组"');
    expect(markup).toContain("选择本地分组");
    expect(markup).toContain("min-w-44");
    expect(markup).toContain('data-slot="combobox-trigger"');
  });

  it("shows existing local groups in the ordinary multi-select without exposing IDs", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingSelect
        upstreamGroupName="pro"
        upstreamPlatform="openai"
        groups={[
          { id: "8", name: "codex", platform: "openai" },
          { id: "9", name: "pro", platform: "openai" },
        ]}
        value={["8", "9"]}
        disabled={false}
        disabledReason={null}
        onValueChange={() => undefined}
      />,
    );

    expect(markup).toContain(">codex<");
    expect(markup).toContain(">pro<");
    expect(markup).not.toContain(">8<");
    expect(markup).not.toContain(">9<");
    expect(markup).not.toContain("已有绑定");
    expect(markup).toContain('role="combobox"');
    expect(markup).not.toContain('disabled=""');
  });

  it("shows the selected local group without exposing its ID", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingSelect
        upstreamGroupName="kiro-power"
        upstreamPlatform="anthropic"
        groups={[{ id: "8", name: "A-kiro逆向", platform: "anthropic" }]}
        value={["8"]}
        disabled={false}
        disabledReason={null}
        onValueChange={() => undefined}
      />,
    );

    expect(markup).toContain("A-kiro逆向");
    expect(markup).not.toContain(">8<");
  });

  it("keeps an existing typed selection available when the upstream platform is missing", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingSelect
        upstreamGroupName="Codex 满血稳定官渠"
        upstreamPlatform={null}
        groups={[
          { id: "25", name: "codex-pro-旗舰", platform: "openai" },
          { id: "22", name: "Gemini", platform: "gemini" },
        ]}
        value={["25"]}
        disabled={false}
        disabledReason={null}
        onValueChange={() => undefined}
      />,
    );

    expect(markup).toContain("codex-pro-旗舰");
    expect(markup).not.toContain("所选分组不可用");
  });

  it("keeps the unavailable reason after the separate status column is removed", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingSelect
        upstreamGroupName="pro"
        upstreamPlatform="openai"
        groups={[]}
        value={[]}
        disabled
        disabledReason="上游余额不足"
        onValueChange={() => undefined}
      />,
    );

    expect(markup).toContain('aria-label="pro 不可添加：上游余额不足"');
  });

  it("shows the local group name and multiplier in account onboarding options", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingOption
        group={{ id: "8", name: "codex-特价", rate_multiplier: "0.15" }}
      />,
    );

    expect(markup).toContain("codex-特价");
    expect(markup).toContain(">0.15<");
    expect(markup).not.toContain("倍率");
    expect(markup).toContain("justify-between");
  });

  it("does not expose local groups from another platform", () => {
    const markup = renderToStaticMarkup(
      <OnboardingGroupBindingSelect
        upstreamGroupName="glm-4.5"
        upstreamPlatform="zhipu"
        groups={[
          { id: "27", name: "国模-平价", platform: "openai" },
          { id: "28", name: "GLM 专用", platform: "zhipu" },
        ]}
        value={["27", "28"]}
        disabled={false}
        disabledReason={null}
        onValueChange={() => undefined}
      />,
    );

    expect(markup).toContain("GLM 专用");
    expect(markup).toContain("所选分组不可用");
    expect(markup).not.toContain("国模-平价");
  });
});
