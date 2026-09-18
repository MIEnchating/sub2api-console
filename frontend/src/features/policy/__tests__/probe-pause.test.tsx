import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, policyDraft, policyPayload } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";
import { probePauseWindowSchema } from "../lib/probe-pause-schema";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function Editor(): ReactElement {
  const [value, setValue] = useState(policyDraft(policy));
  return (
    <>
      <PolicyOperationsEditor
        section="sampling"
        value={value}
        onChange={setValue}
        probesPending={false}
        onProbesEnabledChange={() => undefined}
      />
      <button disabled={!policyPayload(value)}>保存策略</button>
      <output aria-label="暂停配置">
        {JSON.stringify(policyPayload(value)?.advanced_policy?.probe)}
      </output>
    </>
  );
}

describe("自动探活暂停时段", () => {
  it("默认关闭且时间禁用，键盘启用后可保存跨午夜时段", async () => {
    const user = userEvent.setup();
    render(<Editor />);
    const toggle = screen.getByRole("switch", { name: "启用每日探活暂停时段" });
    const start = screen.getByLabelText("暂停开始时间");
    expect(toggle).not.toBeChecked();
    expect(start).toBeDisabled();
    toggle.focus();
    await user.keyboard(" ");
    expect(toggle).toBeChecked();
    expect(start).toBeEnabled();
    fireEvent.change(start, { target: { value: "23:00" } });
    expect(screen.getByLabelText("暂停配置")).toHaveTextContent('"start":"23:00"');
    expect(screen.getByLabelText("暂停配置")).toHaveTextContent('"timezone":"Asia/Shanghai"');
    expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled();
    await user.click(toggle);
    expect(start).toBeDisabled();
    await user.click(toggle);
    expect(start).toHaveValue("23:00");
  });

  it("开始结束时间相同时显示字段错误并禁止保存", async () => {
    const user = userEvent.setup();
    render(<Editor />);
    await user.click(screen.getByRole("switch", { name: "启用每日探活暂停时段" }));
    fireEvent.change(screen.getByLabelText("暂停结束时间"), { target: { value: "00:00" } });
    await waitFor(() =>
      expect(screen.getByLabelText("暂停结束时间")).toHaveAttribute("aria-invalid", "true"),
    );
    expect(screen.getByText("结束时间不能与开始时间相同")).toBeVisible();
    expect(screen.getByRole("button", { name: "保存策略" })).toBeDisabled();
  });

  it.each([
    { start: "24:00" },
    { end: "08:60" },
    { timezone: "Local" },
    { timezone: "Invalid/Zone" },
    { enabled: "true" },
  ])("配置无效时拒绝提交：%j", (invalid) => {
    expect(
      probePauseWindowSchema.safeParse({
        enabled: true,
        start: "23:00",
        end: "08:00",
        timezone: "Asia/Shanghai",
        ...invalid,
      }).success,
    ).toBe(false);
  });
});
