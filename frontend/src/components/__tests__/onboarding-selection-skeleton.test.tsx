import { renderToStaticMarkup } from "react-dom/server";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { OnboardingSelectionSkeleton } from "../onboarding-selection-skeleton";

describe("OnboardingSelectionSkeleton", () => {
  it("加载候选分组时保持与实际表一致的五列和固定操作列", () => {
    render(<OnboardingSelectionSkeleton fillAvailableHeight groupLocked={false} />);
    const table = screen.getByRole("table", { name: "正在加载上游分组" });
    expect(table).toHaveAttribute("data-action-column", "true");
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((column) => column.textContent),
    ).toEqual(["上游分组", "账号成本", "账号类型", "本地分组", "操作"]);
    for (const row of within(table).getAllByRole("row", { name: "正在加载分组" })) {
      expect(within(row).getAllByRole("cell")).toHaveLength(5);
    }
  });
  it.each([true, false])(
    "分组锁定为 %s 时，窄屏字段和汇总允许收缩且控件保持 32px",
    (groupLocked) => {
      render(<OnboardingSelectionSkeleton fillAvailableHeight={false} groupLocked={groupLocked} />);
      const loading = screen.getByRole("status");
      expect(loading).toHaveAttribute("aria-busy", "true");
      const controls = loading.querySelectorAll(
        '[data-onboarding-skeleton="form"] [data-slot="skeleton-control"]',
      );
      expect(controls.length).toBeGreaterThan(0);
      for (const control of controls) expect(control).toHaveClass("h-8", "w-full");
      expect(loading.querySelector('[data-onboarding-skeleton="form"]')).toHaveClass("min-w-0");
    },
  );
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
    expect(markup).toContain("grid-rows-[auto_minmax(12rem,1fr)_auto]");
    expect(markup).toContain("overflow-y-auto");
    expect(markup).toContain("min-w-[800px]");
    expect(markup).toContain("账号类型");
    expect(markup).toContain("本地分组");
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
