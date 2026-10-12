import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { beforeEach, expect, it, vi } from "vitest";

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
});

import { PolicyOperationsEditor, policyDraft, policyPayload } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

function Editor(): ReactElement {
  const [value, setValue] = useState(() => policyDraft(policy));
  return (
    <>
      <PolicyOperationsEditor
        section="health"
        value={value}
        onChange={setValue}
        probesPending={false}
        onProbesEnabledChange={() => undefined}
      />
      <button disabled={!policyPayload(value)}>保存测试策略</button>
    </>
  );
}

it("旧配置默认关闭失效绑定自动删除且每轮最多删除一个", () => {
  render(<Editor />);
  expect(screen.getByRole("switch", { name: "启用失效绑定账号自动删除" })).not.toBeChecked();
  expect(screen.getByRole("spinbutton", { name: "失效绑定每轮最多删除（个）" })).toHaveValue(1);
});

it("键盘启用开关后允许调整删除数量，无效数量阻止保存", async () => {
  const user = userEvent.setup();
  render(<Editor />);
  const toggle = screen.getByRole("switch", { name: "启用失效绑定账号自动删除" });
  toggle.focus();
  await user.keyboard(" ");
  expect(toggle).toBeChecked();
  const input = screen.getByRole("spinbutton", { name: "失效绑定每轮最多删除（个）" });
  fireEvent.change(input, { target: { value: "0" } });
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByRole("button", { name: "保存测试策略" })).toBeDisabled();
  fireEvent.change(input, { target: { value: "3" } });
  expect(input).toHaveAttribute("aria-invalid", "false");
  expect(screen.getByRole("button", { name: "保存测试策略" })).toBeEnabled();
});
