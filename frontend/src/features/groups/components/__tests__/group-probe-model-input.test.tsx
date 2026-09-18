import { fireEvent, screen } from "@testing-library/react";
import { render } from "./dictionary-render";
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
  it("获取模型时仅按钮显示忙碌状态且仍可手动填写", () => {
    render(
      <GroupPolicyEditorFields
        value={value}
        onChange={vi.fn()}
        probeModelsLoading
        onReloadProbeModels={vi.fn()}
      />,
    );
    const button = screen.getByRole("button", { name: "正在获取" });
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "手动输入探活模型" })).toBeEnabled();
  });

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

  it("切换到模型下拉框后隐藏表单节点不产生额外垂直间距", () => {
    render(
      <GroupPolicyEditorFields
        value={value}
        onChange={() => undefined}
        onReloadProbeModels={() => undefined}
        probeModels={{
          group_id: "6",
          group_name: "codex",
          models: ["gpt-5.1-codex"],
          account_count: 1,
          accounts_with_models: 1,
          complete: true,
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "选择模型" }));

    const modeGroup = screen.getByRole("group", { name: "探活模型输入方式" });
    const control = modeGroup.parentElement?.parentElement;
    const reloadButton = screen.getByRole("button", { name: "重新获取组内模型" });

    expect(control).toHaveClass("flex", "flex-col", "gap-1.5");
    expect(control).not.toHaveClass("space-y-1.5");
    expect(reloadButton.parentElement).toHaveClass("sm:items-end");
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

it("组内模型为空时仍可切换选择方式并看到手动输入提示", () => {
  render(
    <GroupPolicyEditorFields
      value={value}
      onChange={() => undefined}
      probeModels={{
        group_id: "6",
        group_name: "codex",
        models: [],
        account_count: 2,
        accounts_with_models: 2,
        complete: true,
      }}
    />,
  );
  const mode = screen.getByRole("button", { name: "选择模型" });
  expect(mode).toBeEnabled();
  fireEvent.click(mode);
  expect(screen.getByRole("combobox", { name: "选择探活模型" })).toHaveTextContent(
    "继承全局默认模型",
  );
  expect(screen.getByRole("status")).toHaveTextContent("暂无组内共同模型，可手动输入探活模型");
});

it("模型加载失败时保留已保存模型并允许手动修改", () => {
  const onChange = vi.fn();
  render(
    <GroupPolicyEditorFields
      value={{ ...value, probe_model: "saved-model" }}
      onChange={onChange}
      probeModelsError
    />,
  );
  expect(screen.getByRole("textbox", { name: "手动输入探活模型" })).toHaveValue("saved-model");
  expect(screen.getByRole("status")).toHaveTextContent("获取组内模型失败");
  fireEvent.change(screen.getByRole("textbox", { name: "手动输入探活模型" }), {
    target: { value: "replacement-model" },
  });
  expect(onChange).toHaveBeenCalledWith({ ...value, probe_model: "replacement-model" });
});

it("继承全局模型时显示实际模型且不写入分组覆盖", () => {
  const onChange = vi.fn();
  render(
    <GroupPolicyEditorFields value={value} onChange={onChange} globalProbeModel="global-model" />,
  );
  expect(screen.getByText("当前继承全局模型：global-model")).toBeVisible();
  expect(screen.getByRole("textbox", { name: "手动输入探活模型" })).toHaveValue("");
  fireEvent.click(screen.getByRole("button", { name: "选择模型" }));
  expect(onChange).not.toHaveBeenCalled();
});

it("定时测试关闭时两种输入方式和模型输入均禁用", () => {
  render(
    <GroupPolicyEditorFields
      value={{ ...value, probe_enabled: false }}
      onChange={() => undefined}
    />,
  );
  expect(screen.getByRole("button", { name: "选择模型" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "手动输入" })).toBeDisabled();
  expect(screen.getByRole("textbox", { name: "手动输入探活模型" })).toBeDisabled();
});

it("全局模型未指定时显示按账号模型回退且不伪造固定模型", () => {
  render(<GroupPolicyEditorFields value={value} onChange={vi.fn()} globalProbeModel={null} />);
  expect(screen.getByText(/全局未指定模型.*各账号已同步的首个可用模型/)).toBeVisible();
});
