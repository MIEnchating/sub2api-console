import { render, screen } from "@testing-library/react";
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
