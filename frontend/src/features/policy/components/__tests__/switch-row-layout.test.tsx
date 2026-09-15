import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { expect, it, vi } from "vitest";

import { PolicySwitchRow } from "../policy-switch-row";

function SwitchRow(): ReactElement {
  const [enabled, setEnabled] = useState(false);
  return (
    <PolicySwitchRow
      label="启用降级"
      description="低分账号压低权重。"
      checked={enabled}
      onCheckedChange={setEnabled}
    />
  );
}

it("窄容器内开关与可换行标签保持横向排列", () => {
  render(<SwitchRow />);
  const toggle = screen.getByRole("switch", { name: "启用降级" });
  expect(toggle.parentElement).toHaveClass("flex", "min-w-0", "items-center", "justify-between");
  expect(toggle.parentElement).not.toHaveClass("flex-col");
  expect(screen.getByText("启用降级").parentElement).toHaveClass("[overflow-wrap:anywhere]");
  expect(screen.getByRole("button", { name: "启用降级说明" })).toBeInTheDocument();
});

it("点击标签及使用空格操作时，开关状态与焦点正确", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const user = userEvent.setup();
  render(<SwitchRow />);
  await user.click(screen.getByText("启用降级"));
  const toggle = screen.getByRole("switch", { name: "启用降级" });
  expect(toggle).toBeChecked();
  toggle.focus();
  await user.keyboard(" ");
  expect(toggle).not.toBeChecked();
  expect(toggle).toHaveFocus();
});

it("读取中禁用开关时，点击标签不会修改状态", async () => {
  const user = userEvent.setup();
  const onCheckedChange = vi.fn();
  render(
    <PolicySwitchRow
      label="启用主动探测"
      description="补充健康样本。"
      checked
      disabled
      onCheckedChange={onCheckedChange}
    />,
  );
  await user.click(screen.getByText("启用主动探测"));
  expect(screen.getByRole("switch", { name: "启用主动探测" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(onCheckedChange).not.toHaveBeenCalled();
});
