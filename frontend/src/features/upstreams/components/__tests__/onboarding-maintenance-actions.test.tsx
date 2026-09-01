import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { OnboardingMaintenanceActions } from "../onboarding-maintenance-actions";

describe("OnboardingMaintenanceActions", () => {
  it("shows revalidation and name repair without account selection", () => {
    const markup = renderToStaticMarkup(
      <OnboardingMaintenanceActions
        accountCount={4}
        pending={false}
        onRevalidate={vi.fn()}
        onRepairNames={vi.fn()}
        onCleanupKeys={vi.fn()}
      />,
    );

    expect(markup).toContain("复验绑定");
    expect(markup).toContain("名称修复");
    expect(markup).toContain("清理无用 Key");
    expect(markup).not.toContain(' disabled=""');
    expect(markup).not.toContain('type="checkbox"');
  });

  it("disables both actions when the current view has no bound account", () => {
    const markup = renderToStaticMarkup(
      <OnboardingMaintenanceActions
        accountCount={0}
        pending={false}
        onRevalidate={vi.fn()}
        onRepairNames={vi.fn()}
        onCleanupKeys={vi.fn()}
      />,
    );

    expect(markup.match(/ disabled=""/g)).toHaveLength(2);
    expect(markup).toContain("清理无用 Key");
  });
});
