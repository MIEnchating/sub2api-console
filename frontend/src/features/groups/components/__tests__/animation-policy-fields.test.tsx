import { screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import type { GroupPolicyOverrideUpdate } from "@/api";
import { GroupPolicyEditorFields } from "../group-policy-editor-fields";
import { render } from "./dictionary-render";

const policy: GroupPolicyOverrideUpdate = {
  enabled: true,
  strategy: "balanced",
  min_pool_size: 1,
  weight_budget: 400,
  balanced_price_ratio: 0.5,
  breaker_enabled: true,
  recovery_enabled: true,
  weights_enabled: true,
  scaling_enabled: false,
  probe_enabled: true,
  probe_interval_seconds: 300,
  probe_model: null,
  animation_enabled: true,
  animation_pass_multiplier: 1.2,
  animation_fail_multiplier: 0.5,
  animation_failure_action: "degrade",
};

it.each([
  ["ignore", "忽略"],
  ["degrade", "适当降级"],
  ["fuse", "熔断账号"],
] as const)("异常处置已保存为 %s 时显示中文选项 %s", (action, label) => {
  render(
    <GroupPolicyEditorFields
      value={{ ...policy, animation_failure_action: action }}
      onChange={() => undefined}
    />,
  );

  const field = screen.getByRole("combobox", { name: "动画异常失败处置" });
  expect(field).toHaveTextContent(label);
  expect(field).not.toHaveTextContent(action);
});

it("倍率和异常处置说明解释对组内调度的影响", () => {
  render(<GroupPolicyEditorFields value={policy} onChange={() => undefined} />);

  const settings = within(screen.getByTestId("group-policy-animation-settings"));
  const passInput = settings.getByRole("spinbutton", { name: "检测通过后：增加分配机会" });
  expect(passInput).toHaveValue(1.2);
  expect(passInput).toHaveAccessibleDescription(/调度会更倾向把请求分配给该账号/);
  const failInput = settings.getByRole("spinbutton", { name: "检测降智后：减少分配机会" });
  expect(failInput).toHaveValue(0.5);
  expect(failInput).toHaveAccessibleDescription(/调度会减少分配给该账号的请求/);
  expect(settings.getByText("推荐 1.2 倍；1 倍不调整")).toBeVisible();
  expect(settings.getByText("推荐 0.7 倍；1 倍不调整")).toBeVisible();
  expect(settings.getByRole("combobox", { name: "动画异常失败处置" })).toHaveAccessibleDescription(
    /上游异常时生效/,
  );
  expect(settings.getByRole("button", { name: "检测通过后：增加分配机会说明" })).toBeEnabled();
  expect(settings.getByRole("button", { name: "异常失败处置说明" })).toBeEnabled();
});
