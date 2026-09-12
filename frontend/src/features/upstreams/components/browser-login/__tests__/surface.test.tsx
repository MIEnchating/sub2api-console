import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { BrowserLoginSession } from "@/api";
import { BrowserSurface } from "../browser-surface";
const session: BrowserLoginSession = {
  id: "session",
  task_id: "task",
  host: "login.example",
  status: "waiting",
  message: "等待登录",
  expires_at: "2030-01-01T00:00:00Z",
  image: "data:image/jpeg;base64,ZnJhbWU=",
  width: 1100,
  height: 760,
};
describe("上游浏览器输入", () => {
  it("画面缩放后点击仍发送对应的上游坐标", () => {
    const onInput = vi.fn().mockResolvedValue(undefined);
    render(<BrowserSurface session={session} disabled={false} onInput={onInput} />);
    const surface = screen.getByRole("button", { name: "上游登录页面" });
    vi.spyOn(surface, "getBoundingClientRect").mockReturnValue({
      x: 10,
      y: 20,
      left: 10,
      top: 20,
      width: 550,
      height: 380,
      right: 560,
      bottom: 400,
      toJSON: () => ({}),
    });
    fireEvent.click(surface, { clientX: 285, clientY: 210 });
    expect(onInput).toHaveBeenCalledWith({ kind: "click", x: 550, y: 380 });
    expect(surface).toHaveFocus();
  });
  it("键盘导航可切换上游输入框且 Tab 不困住控制台焦点", async () => {
    const user = userEvent.setup();
    const onInput = vi.fn().mockResolvedValue(undefined);
    render(<BrowserSurface session={session} disabled={false} onInput={onInput} />);
    screen.getByRole("button", { name: "上游登录页面" }).focus();
    await user.keyboard("a{Enter}");
    expect(onInput).toHaveBeenCalledWith({ kind: "text", text: "a" });
    expect(onInput).toHaveBeenCalledWith({ kind: "key", key: "Enter", shift: false });
    await user.tab();
    expect(screen.getByRole("button", { name: "上一个输入框" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "下一个输入框" }));
    expect(onInput).toHaveBeenCalledWith({ kind: "key", key: "Tab" });
  });
  it("输入中文后发送并清除本地文字", async () => {
    const user = userEvent.setup();
    const onInput = vi.fn().mockResolvedValue(undefined);
    render(<BrowserSurface session={session} disabled={false} onInput={onInput} />);
    const field = screen.getByLabelText("发送到上游当前输入框的文字");
    await user.type(field, "测试用户");
    await user.click(screen.getByRole("button", { name: "发送文字" }));
    expect(onInput).toHaveBeenCalledWith({ kind: "text", text: "测试用户" });
    expect(field).toHaveValue("");
  });
  it("复核期间禁用画面与输入操作", () => {
    const onInput = vi.fn().mockResolvedValue(undefined);
    render(<BrowserSurface session={session} disabled onInput={onInput} />);
    const surface = screen.getByRole("button", { name: "上游登录页面" });
    expect(surface).toHaveAttribute("aria-disabled", "true");
    expect(surface).toHaveAttribute("tabindex", "-1");
    fireEvent.click(surface);
    fireEvent.keyDown(surface, { key: "a" });
    expect(screen.getByRole("button", { name: "下一个输入框" })).toBeDisabled();
    expect(onInput).not.toHaveBeenCalled();
  });
});
