import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { OnboardingHeadingActions } from "../onboarding-heading-actions";

describe("OnboardingHeadingActions", () => {
  it("provides an explicit return action on the account onboarding page", () => {
    const markup = renderToStaticMarkup(<OnboardingHeadingActions onBack={vi.fn()} />);

    expect(markup).toContain("返回上游管理");
    expect(markup).toContain('type="button"');
    expect(markup).toContain("新建 Key");
    expect(markup).toContain("上一个上游");
    expect(markup).toContain("下一个上游");
    expect(markup.match(/<button[^>]*disabled=""/g)).toHaveLength(2);
  });

  it("labels available adjacent upstream actions with their destinations", () => {
    const markup = renderToStaticMarkup(
      <OnboardingHeadingActions
        onBack={vi.fn()}
        previousUpstream={{ label: "上游 A", onSelect: vi.fn() }}
        nextUpstream={{ label: "上游 C", onSelect: vi.fn() }}
      />,
    );

    expect(markup).toContain('aria-label="上一个上游：上游 A"');
    expect(markup).toContain('aria-label="下一个上游：上游 C"');
    expect(markup).not.toContain('disabled=""');
  });
});
