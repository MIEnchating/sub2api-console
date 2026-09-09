import type { ComponentProps } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UpstreamsPageActions } from "../upstreams-page-actions";

function actions(): ComponentProps<typeof UpstreamsPageActions> {
  return {
    refreshing: false,
    maintenancePending: false,
    syncPending: false,
    auditPending: false,
    auditDisabled: false,
    onRefresh: vi.fn(),
    onAdd: vi.fn(),
    onHistory: vi.fn(),
    onBalanceSync: vi.fn(),
    onNameRepair: vi.fn(),
    onGroupSync: vi.fn(),
    onGroupAudit: vi.fn(),
    onSync: vi.fn(),
  };
}
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

describe("上游顶部维护入口", () => {
  it.each([
    { label: "同步分组", callback: "onGroupSync" as const },
    { label: "名称修复", callback: "onNameRepair" as const },
    { label: "核对分组绑定", callback: "onGroupAudit" as const },
  ])("从维护菜单选择 $label 时执行原有操作", async (fixture) => {
    const user = userEvent.setup();
    const props = actions();
    render(<UpstreamsPageActions {...props} />);
    expect(screen.getAllByRole("button")).toHaveLength(3);
    screen.getByRole("button", { name: "上游维护" }).focus();
    await user.keyboard("{Enter}");
    await user.click(await screen.findByRole("menuitem", { name: fixture.label }));
    expect(props[fixture.callback]).toHaveBeenCalledOnce();
  });

  it("各维护任务进行中时禁用相关操作，统计变化仍可查看", async () => {
    const user = userEvent.setup();
    render(
      <UpstreamsPageActions
        {...actions()}
        maintenancePending
        syncPending
        auditPending
        auditDisabled
      />,
    );
    screen.getByRole("button", { name: "上游维护" }).focus();
    await user.keyboard("{Enter}");
    for (const label of ["同步上游", "同步余额", "名称修复", "同步分组", "核对中…"]) {
      expect(await screen.findByRole("menuitem", { name: label })).toHaveAttribute(
        "aria-disabled",
        "true",
      );
    }
    expect(screen.getByRole("menuitem", { name: "统计变化" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });
});
