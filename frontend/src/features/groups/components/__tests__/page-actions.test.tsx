import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GroupsPageActions } from "../groups-page-actions";

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

describe("分组顶部维护入口", () => {
  it.each([
    { name: "没有可处理分组", disabled: false, targetCount: 0 },
    { name: "加载、策略不可用或写入中", disabled: true, targetCount: 1 },
  ])("$name 时可以查看维护操作但不能提交", async (fixture) => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(
      <GroupsPageActions
        refreshing={false}
        selectedCount={0}
        disabled={fixture.disabled}
        targetCount={fixture.targetCount}
        onRefresh={vi.fn()}
        onAction={onAction}
      />,
    );
    screen.getByRole("button", { name: "分组维护" }).focus();
    await user.keyboard("{Enter}");
    for (const action of await screen.findAllByRole("menuitem")) {
      expect(action).toHaveAttribute("aria-disabled", "true");
      await user.click(action);
    }
    expect(onAction).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "刷新分组" })).toBeEnabled();
  });
});
