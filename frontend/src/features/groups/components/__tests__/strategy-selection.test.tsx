import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { expect, it } from "vitest";
import {
  GroupPolicyEditorFields,
  type GroupPolicyOverrideDraft,
} from "../group-policy-editor-fields";

function Editor() {
  const [value, setValue] = useState<GroupPolicyOverrideDraft>({
    enabled: true,
    strategy: null,
    min_pool_size: 1,
    weight_budget: 400,
    balanced_price_ratio: 0.5,
    breaker_enabled: true,
    recovery_enabled: true,
    weights_enabled: true,
    scaling_enabled: false,
    probe_enabled: true,
    probe_interval_seconds: 300,
    probe_model: "saved-model",
  });
  return <GroupPolicyEditorFields value={value} onChange={setValue} globalStrategy="speed_first" />;
}

it("继承策略时选中全局默认并显示当前全局策略", () => {
  render(<Editor />);
  expect(screen.getByRole("radio", { name: "全局默认" })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByRole("radio", { name: "速度优先" })).toHaveAttribute("aria-checked", "false");
  expect(screen.getByText(/继承全局默认，随全局策略变化（当前：速度优先）/)).toBeVisible();
});

it("通过键盘选择策略后可回到全局默认且保留探活模型", async () => {
  const user = userEvent.setup();
  render(<Editor />);
  const price = screen.getByRole("radio", { name: "价格优先" });
  price.focus();
  await user.keyboard("{Enter}");
  expect(price).toHaveAttribute("aria-checked", "true");
  const inherited = screen.getByRole("radio", { name: "全局默认" });
  inherited.focus();
  await user.keyboard(" ");
  expect(inherited).toHaveAttribute("aria-checked", "true");
  expect(price).toHaveAttribute("aria-checked", "false");
  expect(screen.getByRole("textbox", { name: "手动输入探活模型" })).toHaveValue("saved-model");
});
