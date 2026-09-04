import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { OnboardingSelectionSkeleton } from "../onboarding-selection-skeleton";

describe("OnboardingSelectionSkeleton", () => {
  it("preserves the complete Host onboarding layout while loading", () => {
    const markup = renderToStaticMarkup(
      <OnboardingSelectionSkeleton fillAvailableHeight groupLocked={false} />,
    );

    expect(markup).toContain('role="status"');
    expect(markup).toContain('aria-label="正在获取上游信息"');
    expect(markup).toContain('data-onboarding-skeleton="summary"');
    expect(markup).toContain('data-onboarding-skeleton="form"');
    expect(markup).toContain('data-onboarding-skeleton="groups"');
    expect(markup).not.toContain('data-onboarding-skeleton="action"');
    expect(markup).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(markup).toContain("min-w-[1120px]");
    expect(markup).toContain("平台");
    expect(markup).toContain("本地分组");
    expect(markup).toContain("状态");
    expect(markup).not.toContain("绑定到本地分组");
    expect(markup).toContain("操作");
    expect(markup.indexOf('data-onboarding-skeleton="groups"')).toBeLessThan(
      markup.indexOf('data-onboarding-skeleton="form"'),
    );
  });

  it("uses a locked-group summary instead of a candidate table", () => {
    const markup = renderToStaticMarkup(
      <OnboardingSelectionSkeleton fillAvailableHeight={false} groupLocked />,
    );

    expect(markup).toContain('data-onboarding-skeleton="locked-group"');
    expect(markup).not.toContain('data-onboarding-skeleton="groups"');
    expect(markup).toContain('data-onboarding-skeleton="action"');
  });
});
