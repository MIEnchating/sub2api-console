import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, policyDraft, policyPayload } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

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
        probesEnabled
        probesPending={false}
        onProbesEnabledChange={() => undefined}
      />
      <output aria-label="探测策略">
        {JSON.stringify(policyPayload(value)?.advanced_policy?.probe)}
      </output>
    </>
  );
}

describe("持续评估候选速度", () => {
  it("默认启用且键盘关闭后保存独立配置，保留常规流量跳过设置", async () => {
    const user = userEvent.setup();
    render(<Editor />);
    const toggle = screen.getByRole("switch", { name: "持续评估候选速度" });
    expect(toggle).toBeChecked();
    toggle.focus();
    await user.keyboard(" ");
    expect(toggle).not.toBeChecked();
    expect(screen.getByLabelText("探测策略")).toHaveTextContent(
      '"performance_exploration_enabled":false',
    );
    expect(screen.getByRole("switch", { name: "有新鲜流量时跳过探测" })).toBeChecked();
    await user.click(toggle);
    expect(screen.getByLabelText("探测策略")).toHaveTextContent(
      '"performance_exploration_enabled":true',
    );
  });
});
