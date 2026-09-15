import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, policyDraft, policyPayload } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

type PolicyDraft = ReturnType<typeof policyDraft>;

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function Editor(props: {
  draft?: PolicyDraft;
  onChange?: (value: PolicyDraft) => void;
}): ReactElement {
  const [value, setValue] = useState(props.draft ?? policyDraft(policy));
  return (
    <PolicyOperationsEditor
      section="routing"
      value={value}
      onChange={(next) => {
        setValue(next);
        props.onChange?.(next);
      }}
      probesPending={false}
      onProbesEnabledChange={() => undefined}
    />
  );
}

describe("成本墙配置", () => {
  it("未配置时默认启用拦截、保底和停止自动探活", () => {
    render(<Editor />);
    expect(screen.getByRole("switch", { name: "启用成本墙拦截" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "无可用账号时允许保底" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "拦截期间停止自动探活" })).toBeChecked();
  });

  it("独立关闭保底和自动探活限制后保存各自配置", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Editor onChange={onChange} />);
    await user.click(screen.getByRole("switch", { name: "无可用账号时允许保底" }));
    await user.click(screen.getByRole("switch", { name: "拦截期间停止自动探活" }));
    expect(screen.getByRole("switch", { name: "启用成本墙拦截" })).toBeChecked();
    expect(policyPayload(onChange.mock.lastCall![0] as PolicyDraft)?.advanced_policy).toMatchObject(
      {
        cost_wall: { fallback_enabled: false, stop_auto_probe: false },
      },
    );
  });

  it("总开关通过键盘关闭时禁用子开关，再开启后保留已保存选择", async () => {
    const user = userEvent.setup();
    const draft = policyDraft(policy);
    draft.advanced_policy.cost_wall = {
      enabled: true,
      fallback_enabled: false,
      stop_auto_probe: true,
    };
    const onChange = vi.fn();
    render(<Editor draft={draft} onChange={onChange} />);
    const enabled = screen.getByRole("switch", { name: "启用成本墙拦截" });
    const fallback = screen.getByRole("switch", { name: "无可用账号时允许保底" });
    const probe = screen.getByRole("switch", { name: "拦截期间停止自动探活" });
    enabled.focus();
    await user.keyboard(" ");
    expect(enabled).not.toBeChecked();
    expect(enabled).toHaveFocus();
    expect(fallback).toHaveAttribute("aria-disabled", "true");
    expect(probe).toHaveAttribute("aria-disabled", "true");
    await user.click(fallback);
    expect(fallback).not.toBeChecked();
    enabled.focus();
    expect(policyPayload(onChange.mock.lastCall![0] as PolicyDraft)?.advanced_policy).toMatchObject(
      {
        cost_wall: { enabled: false, fallback_enabled: false, stop_auto_probe: true },
      },
    );
    await user.keyboard(" ");
    expect(fallback).not.toHaveAttribute("aria-disabled", "true");
    expect(fallback).not.toBeChecked();
    expect(probe).not.toHaveAttribute("aria-disabled", "true");
    expect(probe).toBeChecked();
  });

  it.each(["enabled", "fallback_enabled", "stop_auto_probe"])("%s 非布尔值时拒绝保存", (field) => {
    const draft = policyDraft(policy);
    draft.advanced_policy.cost_wall = { [field]: "false" };
    expect(policyPayload(draft)).toBeNull();
  });
});
