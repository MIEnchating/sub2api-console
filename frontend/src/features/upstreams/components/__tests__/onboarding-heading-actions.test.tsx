import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { OnboardingHeadingActions } from "../onboarding-heading-actions";

describe("OnboardingHeadingActions", () => {
  it("provides an explicit return action on the account onboarding page", () => {
    const markup = renderToStaticMarkup(<OnboardingHeadingActions onBack={vi.fn()} />);

    expect(markup).toContain("返回上游管理");
    expect(markup).toContain('type="button"');
    expect(markup).toContain("新建 Key");
  });
});
