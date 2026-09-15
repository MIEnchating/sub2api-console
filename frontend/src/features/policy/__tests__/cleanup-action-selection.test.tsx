import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, policyDraft } from "@/App";
import { policy } from "@/features/accounts/__tests__/fixtures";

type PolicyDraft = ReturnType<typeof policyDraft>;

beforeEach(() => {
  const getComputedStyle = window.getComputedStyle;
  // JSDOM 没有原生选择器布局，隐藏 select 的 UA 样式会递归匹配。
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudoElement) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return getComputedStyle(element, pseudoElement);
  });
});

afterEach(() => vi.restoreAllMocks());

function CleanupEditor(props: {
  action?: string;
  onChange?: (value: PolicyDraft) => void;
}): ReactElement {
  const [value, setValue] = useState(() =>
    policyDraft({
      ...policy,
      advanced_policy: {
        cleanup: { enabled: false, action: props.action },
      },
    }),
  );

  return (
    <PolicyOperationsEditor
      section="health"
      value={value}
      probesPending={false}
      onProbesEnabledChange={() => undefined}
      onChange={(next) => {
        setValue(next);
        props.onChange?.(next);
      }}
    />
  );
}

const actions = [
  ["none", "仅停止调度，不额外处置"],
  ["pause", "暂停调度"],
  ["disable", "停用账号"],
  ["delete", "删除账号"],
] as const;

describe("认证失效处置动作", () => {
  it.each(actions)("已保存 %s 时，菜单展开前后均显示中文标签 %s", (action, label) => {
    render(<CleanupEditor action={action} />);

    const trigger = screen.getByRole("combobox", { name: "处置动作" });
    expect(trigger).toHaveTextContent(label);
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(trigger);
    expect(screen.getByRole("option", { name: label })).toHaveAttribute("aria-selected", "true");
    expect(trigger).toHaveTextContent(label);
  });

  it("未配置动作时默认显示暂停调度", () => {
    render(<CleanupEditor />);

    expect(screen.getByRole("combobox", { name: "处置动作" })).toHaveTextContent("暂停调度");
  });

  it.each(actions)("点击选择 %s 时回显 %s，并将协议值写入草稿", async (action, label) => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<CleanupEditor action={action === "pause" ? "none" : "pause"} onChange={onChange} />);
    const trigger = screen.getByRole("combobox", { name: "处置动作" });

    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    await user.click(screen.getByRole("option", { name: label }));

    expect(trigger).toHaveTextContent(label);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        advanced_policy: expect.objectContaining({
          cleanup: { enabled: false, action },
        }),
      }),
    );
  });

  it("通过键盘选择停用账号后显示中文标签并保留焦点", async () => {
    // JSDOM 尚未实现 Base UI 键盘选择所需的 PointerEvent。
    vi.stubGlobal("PointerEvent", MouseEvent);
    const user = userEvent.setup();
    render(<CleanupEditor action="pause" />);
    const trigger = screen.getByRole("combobox", { name: "处置动作" });
    trigger.focus();

    await user.keyboard("{Enter}");
    expect(screen.getByRole("option", { name: "暂停调度" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.keyboard("{ArrowDown}{Enter}");

    expect(trigger).toHaveTextContent("停用账号");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
  });

  it("遇到未识别的动作时保留原值，不误显示为暂停调度", () => {
    render(<CleanupEditor action="unsupported-action" />);

    expect(screen.getByRole("combobox", { name: "处置动作" })).toHaveTextContent(
      "unsupported-action",
    );
  });
});
