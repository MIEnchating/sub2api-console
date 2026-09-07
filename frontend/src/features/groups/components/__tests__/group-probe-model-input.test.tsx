import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { GroupPolicyOverrideUpdate } from "../../../../api";
import { GroupPolicyEditorFields } from "../group-policy-editor-fields";

const value: GroupPolicyOverrideUpdate = {
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
};

describe("分组探活模型输入", () => {
  it("有组内模型时仍可手动输入任意模型", () => {
    const onChange = vi.fn();
    render(
      <GroupPolicyEditorFields
        value={value}
        onChange={onChange}
        probeModels={{
          group_id: "6",
          group_name: "codex",
          models: ["gpt-5.1-codex", "gpt-5.2"],
          account_count: 2,
          accounts_with_models: 2,
          complete: true,
        }}
      />,
    );

    fireEvent.change(screen.getByRole("textbox", { name: "手动输入探活模型" }), {
      target: { value: "custom-probe-model" },
    });

    expect(onChange).toHaveBeenCalledWith({ ...value, probe_model: "custom-probe-model" });
  });

  it("有组内模型时可切换到项目下拉框", () => {
    const onChange = vi.fn();
    render(
      <GroupPolicyEditorFields
        value={value}
        onChange={onChange}
        probeModels={{
          group_id: "6",
          group_name: "codex",
          models: ["gpt-5.1-codex", "gpt-5.2"],
          account_count: 2,
          accounts_with_models: 2,
          complete: true,
        }}
      />,
    );
    const selectMode = screen.getByRole("button", { name: "选择模型" });
    expect(selectMode).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(selectMode);
    const select = screen.getByRole("combobox", { name: "选择探活模型" });

    expect(selectMode).toHaveAttribute("aria-pressed", "true");
    expect(select).toHaveTextContent("继承全局默认模型");
    expect(select).toHaveAttribute("data-slot", "select-trigger");
    expect(screen.queryByRole("textbox", { name: "手动输入探活模型" })).not.toBeInTheDocument();
  });

  it("清空模型后恢复继承全局默认", () => {
    const onChange = vi.fn();
    render(
      <GroupPolicyEditorFields value={{ ...value, probe_model: "gpt-5.2" }} onChange={onChange} />,
    );

    fireEvent.change(screen.getByRole("textbox", { name: "手动输入探活模型" }), {
      target: { value: "" },
    });

    expect(onChange).toHaveBeenCalledWith({ ...value, probe_model: null });
  });
});
