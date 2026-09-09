import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PricingCatalogActions } from "../pricing-catalog-actions";

beforeEach(() => {
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});
afterEach(() => vi.restoreAllMocks());

describe("价格顶部维护入口", () => {
  it("未加载价格或备份、配置无效或任务运行中时保留操作的禁用状态", async () => {
    const user = userEvent.setup();
    const onBackup = vi.fn();
    const onRestore = vi.fn();
    render(
      <PricingCatalogActions
        previewDisabled
        backupDisabled
        restoreDisabled
        onPreview={vi.fn()}
        onHistory={vi.fn()}
        onBackup={onBackup}
        onRestore={onRestore}
      />,
    );
    expect(screen.getByRole("button", { name: "查看账号调整明细" })).toBeDisabled();
    screen.getByRole("button", { name: "价格维护" }).focus();
    await user.keyboard("{Enter}");
    for (const label of ["创建备份", "从备份还原"]) {
      const action = await screen.findByRole("menuitem", { name: label });
      expect(action).toHaveAttribute("aria-disabled", "true");
      await user.click(action);
    }
    expect(onBackup).not.toHaveBeenCalled();
    expect(onRestore).not.toHaveBeenCalled();
    expect(screen.getByRole("menuitem", { name: "变更记录" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });
});
