import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, policyDraft, policyPayload } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const original = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudo) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return original(element, pseudo);
  });
});

function Editor(props: {
  durationMinutes?: number;
  onSave?: (value: unknown) => void;
}): ReactElement {
  const [value, setValue] = useState(() =>
    policyDraft({
      ...policy,
      advanced_policy: {
        ...policy.advanced_policy,
        ...(props.durationMinutes === undefined
          ? {}
          : { abnormal_cleanup: { duration_minutes: props.durationMinutes } }),
      },
    }),
  );
  return (
    <>
      <PolicyOperationsEditor
        section="health"
        value={value}
        onChange={setValue}
        probesPending={false}
        onProbesEnabledChange={() => undefined}
      />
      <button disabled={!policyPayload(value)} onClick={() => props.onSave?.(policyPayload(value))}>
        保存测试策略
      </button>
    </>
  );
}

it("旧配置默认关闭长期异常处置，并提供一天观察时长与分组保底", () => {
  render(<Editor />);
  const card = within(screen.getByRole("region", { name: "长期异常账号处置" }));
  expect(card.getByRole("switch", { name: "启用长期异常账号处置" })).not.toBeChecked();
  expect(card.getByRole("spinbutton", { name: "异常持续时长（天）" })).toHaveValue(1);
  expect(card.getByRole("switch", { name: "长期异常处置保留分组最后一个账号" })).toBeChecked();
  expect(card.getByRole("combobox", { name: "长期异常处置动作" })).toHaveTextContent("暂停调度");
});

it("开关支持键盘切换，修改时长和动作只更新长期异常配置", async () => {
  const user = userEvent.setup();
  render(<Editor />);
  const enabled = screen.getByRole("switch", { name: "启用长期异常账号处置" });
  enabled.focus();
  await user.keyboard(" ");
  expect(enabled).toBeChecked();
  const duration = screen.getByRole("spinbutton", { name: "异常持续时长（天）" });
  fireEvent.change(duration, { target: { value: "3" } });
  expect(duration).toHaveValue(3);
  const action = screen.getByRole("combobox", { name: "长期异常处置动作" });
  fireEvent.click(action);
  await user.click(screen.getByRole("option", { name: "停用账号" }));
  expect(action).toHaveTextContent("停用账号");
  expect(action).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByRole("combobox", { name: "处置动作" })).toHaveTextContent("暂停调度");
});

it.each(["", "0", "-1", "0.0001", "366"])(
  "时长输入 %s 时标记无效并阻止保存，修正后恢复",
  (value) => {
    render(<Editor />);
    const duration = screen.getByRole("spinbutton", { name: "异常持续时长（天）" });
    fireEvent.change(duration, { target: { value } });
    expect(duration).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("button", { name: "保存测试策略" })).toBeDisabled();
    fireEvent.change(duration, { target: { value: "2" } });
    expect(duration).toHaveAttribute("aria-invalid", "false");
    expect(screen.getByRole("button", { name: "保存测试策略" })).toBeEnabled();
  },
);

it.each([
  [4320, 3],
  [720, 0.5],
  [120, 120 / 1440],
])("已有 %s 分钟配置按 %s 天回显，直接保存不改变时长", async (minutes, days) => {
  const onSave = vi.fn();
  render(<Editor durationMinutes={minutes} onSave={onSave} />);
  expect(screen.getByRole("spinbutton", { name: "异常持续时长（天）" })).toHaveValue(days);
  await userEvent.click(screen.getByRole("button", { name: "保存测试策略" }));
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({
      advanced_policy: expect.objectContaining({ abnormal_cleanup: { duration_minutes: minutes } }),
    }),
  );
});

it.each([
  [3, 4320],
  [0.5, 720],
  [0.7, 1008],
  [365, 525600],
])("输入 %s 天保存为 %s 分钟，不改变实际处置时长", async (days, minutes) => {
  const onSave = vi.fn();
  render(<Editor onSave={onSave} />);
  fireEvent.change(screen.getByRole("spinbutton", { name: "异常持续时长（天）" }), {
    target: { value: String(days) },
  });
  await userEvent.click(screen.getByRole("button", { name: "保存测试策略" }));
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({
      advanced_policy: expect.objectContaining({ abnormal_cleanup: { duration_minutes: minutes } }),
    }),
  );
});
