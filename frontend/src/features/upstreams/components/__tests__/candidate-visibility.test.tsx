import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";

import { defaultOnlyShowEnabledOnboardingGroups } from "../../lib/onboarding-candidate-visibility";
import { OnboardingCandidateVisibilityFilter } from "../onboarding-candidate-visibility-filter";

beforeAll(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterAll(() => vi.unstubAllGlobals());

function VisibilityFilterHarness() {
  const [onlyShowEnabled, setOnlyShowEnabled] = useState(defaultOnlyShowEnabledOnboardingGroups);

  return (
    <OnboardingCandidateVisibilityFilter
      onlyShowEnabled={onlyShowEnabled}
      hiddenCount={2}
      onOnlyShowEnabledChange={setOnlyShowEnabled}
    />
  );
}

describe("账号添加候选分组显示开关", () => {
  it("默认开启仅显示启用分组并说明隐藏数量", () => {
    render(<VisibilityFilterHarness />);

    expect(screen.getByRole("switch", { name: "仅显示启用分组" })).toBeChecked();
    expect(screen.getByText("已隐藏 2 个未启用分组")).toBeVisible();
  });

  it("点击开关后允许显示未启用分组", async () => {
    const user = userEvent.setup();
    render(<VisibilityFilterHarness />);

    const visibilitySwitch = screen.getByRole("switch", { name: "仅显示启用分组" });
    await user.click(visibilitySwitch);

    expect(visibilitySwitch).not.toBeChecked();
    expect(screen.queryByText("已隐藏 2 个未启用分组")).not.toBeInTheDocument();
  });
});
