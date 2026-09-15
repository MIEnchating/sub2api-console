import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { expect, it } from "vitest";

import {
  PolicyCategoryNavigation,
  type PolicyCategory,
} from "../components/policy-category-navigation";

function NavigationFixture(): ReactElement {
  const [value, setValue] = useState<PolicyCategory>("routing");
  return <PolicyCategoryNavigation value={value} onChange={setValue} />;
}

it("窄屏使用两列分类，宽屏使用四列且每项允许收缩换行", () => {
  render(<NavigationFixture />);

  expect(screen.getByRole("tablist", { name: "策略分类" })).toHaveClass(
    "grid",
    "grid-cols-2",
    "sm:grid-cols-4",
    "items-stretch",
  );
  for (const tab of screen.getAllByRole("tab")) {
    expect(tab).toHaveClass("min-w-0", "whitespace-normal", "h-auto", "py-2", "lg:py-3");
    expect(within(tab).getByText(tab.getAttribute("aria-label")!)).toHaveClass("break-words");
  }
});

it("窄屏隐藏分类说明以压缩导航，桌面显示可换行说明且保留可访问描述", () => {
  render(<NavigationFixture />);

  const routing = screen.getByRole("tab", { name: "调度与写入" });
  const description = within(routing).getByText("默认策略、权重与执行");
  expect(description).toHaveClass("hidden", "lg:block", "break-words");
  expect(routing).toHaveAccessibleDescription("默认策略、权重与执行");
});

it("点击分类后仅当前分类呈现选中强调并可通过 Tab 聚焦", async () => {
  const user = userEvent.setup();
  render(<NavigationFixture />);
  const routing = screen.getByRole("tab", { name: "调度与写入" });
  const health = screen.getByRole("tab", { name: "健康与处置" });

  await user.click(health);

  expect(health).toHaveAttribute("aria-selected", "true");
  expect(health).toHaveAttribute("tabindex", "0");
  expect(health).toHaveClass("border-primary/20");
  expect(health).toHaveAttribute("aria-controls", "policy-panel-health");
  expect(routing).toHaveAttribute("aria-selected", "false");
  expect(routing).toHaveAttribute("tabindex", "-1");
  expect(routing).not.toHaveClass("border-primary/20");
  expect(screen.getAllByRole("tab", { selected: true })).toHaveLength(1);
});

it("方向键及首尾键切换分类时，焦点与选中状态同步", async () => {
  const user = userEvent.setup();
  render(<NavigationFixture />);
  const routing = screen.getByRole("tab", { name: "调度与写入" });
  routing.focus();
  await user.keyboard("{ArrowRight}");
  const health = screen.getByRole("tab", { name: "健康与处置" });
  expect(health).toHaveFocus();
  expect(health).toHaveAttribute("aria-selected", "true");
  expect(routing).toHaveAttribute("aria-selected", "false");
  await user.keyboard("{End}");
  expect(screen.getByRole("tab", { name: "守护范围" })).toHaveFocus();
  await user.keyboard("{Home}");
  expect(routing).toHaveFocus();
  expect(routing).toHaveAttribute("aria-selected", "true");
  expect(routing).toHaveAttribute("aria-controls", "policy-panel-routing");
});
