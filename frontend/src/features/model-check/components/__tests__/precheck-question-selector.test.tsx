import { useState, type ReactElement } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { PrecheckQuestionID } from "@/api";
import { PrecheckQuestionSelector } from "../precheck-question-selector";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function Selector(): ReactElement {
  const [value, setValue] = useState<PrecheckQuestionID[]>(["candy"]);
  return <PrecheckQuestionSelector value={value} onChange={setValue} />;
}

it("键盘可以取消并重新选择唯一检测题，关闭题目菜单后焦点返回入口", async () => {
  render(<Selector />);
  const user = userEvent.setup();
  await user.tab();
  await user.keyboard("{Enter}");
  const all = await screen.findByRole("checkbox", { name: "全选" });
  expect(all).toHaveFocus();
  await user.keyboard(" ");
  expect(all).not.toBeChecked();
  await user.tab();
  expect(screen.getByRole("checkbox", { name: "糖果题" })).toHaveFocus();
  await user.keyboard(" ");
  expect(all).toBeChecked();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("button", { name: "选择前置检测题目" })).toHaveFocus();
  expect(screen.getByRole("button", { name: "选择前置检测题目" })).toHaveTextContent(
    "检测题目（1）",
  );
});

it("提交期间题目选择禁用且不能打开菜单", async () => {
  render(<PrecheckQuestionSelector value={["candy"]} onChange={() => {}} disabled />);
  const button = screen.getByRole("button", { name: "选择前置检测题目" });
  expect(button).toBeDisabled();
  await userEvent.click(button);
  expect(screen.queryByRole("dialog", { name: "前置检测题目" })).not.toBeInTheDocument();
});
