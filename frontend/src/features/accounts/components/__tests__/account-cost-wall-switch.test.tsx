import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { AccountCostWallSwitch } from "../account-cost-wall-switch";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function Editor(props: { enabled?: boolean; disabled?: boolean }) {
  const [enabled, setEnabled] = useState(props.enabled === true);
  return (
    <AccountCostWallSwitch
      accountId="41"
      enabled={enabled}
      disabled={props.disabled}
      onCheckedChange={setEnabled}
    />
  );
}
it("默认关闭，键盘开启只更新草稿并说明保存后生效", async () => {
  const save = vi.spyOn(api, "setAccountIgnoreCostWall");
  render(<Editor />);
  const control = screen.getByRole("switch", { name: "无视成本墙" });
  expect(control).not.toBeChecked();
  expect(control).toHaveAccessibleDescription(/点击保存后生效/);
  expect(control).toHaveAccessibleDescription(/停止无利润／亏损流量告警及其恢复通知/);
  await userEvent.tab();
  expect(control).toHaveFocus();
  await userEvent.keyboard(" ");
  expect(control).toBeChecked();
  expect(save).not.toHaveBeenCalled();
});
it("已开启的账号回显开关，点击后仅将草稿改为关闭", async () => {
  const save = vi.spyOn(api, "setAccountIgnoreCostWall");
  render(<Editor enabled />);
  const control = screen.getByRole("switch", { name: "无视成本墙" });
  expect(control).toBeChecked();
  await userEvent.click(control);
  expect(control).not.toBeChecked();
  expect(save).not.toHaveBeenCalled();
});
it("保存期间禁用开关，点击不会改变草稿", async () => {
  render(<Editor disabled />);
  const control = screen.getByRole("switch", { name: "无视成本墙" });
  expect(control).toHaveAttribute("aria-disabled", "true");
  await userEvent.click(control);
  expect(control).not.toBeChecked();
});
