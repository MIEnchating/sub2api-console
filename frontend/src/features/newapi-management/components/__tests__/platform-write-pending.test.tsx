import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { NewAPIPlatformDialog } from "../platform-dialog";

it("平台验证期间锁定编辑与关闭入口，失败结束后恢复操作", () => {
  const close = vi.fn();
  const submit = vi.fn();
  const props = { open: true, platform: null, onOpenChange: close, onSubmit: submit };
  const view = render(<NewAPIPlatformDialog {...props} pending />);
  for (const label of ["平台名称", "平台地址", "User ID", "Admin Key"])
    expect(screen.getByLabelText(label)).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  expect(screen.queryByRole("button", { name: "关闭" })).not.toBeInTheDocument();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
  view.rerender(<NewAPIPlatformDialog {...props} pending={false} />);
  expect(screen.getByLabelText("平台名称")).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(close).toHaveBeenCalledWith(false);
});
