import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchOAuthStart } from "../components/workbench-oauth-start";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("启动设置作为独立区域展示，键盘可启用检查点并提交原配置", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const onStart = vi.fn();
  render(<WorkbenchOAuthStart disabled={false} retry={false} onStart={onStart} />);
  const section = screen.getByRole("region", { name: "授权启动设置" });
  const user = userEvent.setup();
  const recovery = within(section).getByRole("checkbox", { name: /自动保存私有登录检查点/ });
  recovery.focus();
  await user.keyboard(" ");
  expect(recovery).toBeChecked();
  await user.click(within(section).getByRole("button", { name: "开始授权登录" }));
  expect(onStart).toHaveBeenCalledWith({ recovery_enabled: true });
});

it("启动设置被禁用时，代理、辅助选项和主操作保持一致的禁用状态", () => {
  render(<WorkbenchOAuthStart disabled retry={false} onStart={vi.fn()} />);
  expect(screen.getByLabelText("登录代理")).toBeDisabled();
  for (const option of screen.getAllByRole("checkbox")) {
    expect(option).toHaveAttribute("aria-disabled", "true");
  }
  expect(screen.getByRole("button", { name: "开始授权登录" })).toBeDisabled();
});
