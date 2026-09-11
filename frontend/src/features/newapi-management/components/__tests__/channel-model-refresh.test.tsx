import userEvent from "@testing-library/user-event";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { NewAPIChannelModelDialog } from "../channel-model-dialog";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

it("刷新已有上游模型时保留列表和勾选，失败后不能确认旧目录", () => {
  const props = {
    open: true,
    models: ["model-a"],
    selected: ["model-a"],
    pending: false,
    error: "",
    onOpenChange: vi.fn(),
    onSelectedChange: vi.fn(),
    onConfirm: vi.fn(),
  };
  const view = render(<NewAPIChannelModelDialog {...props} />);
  const checkbox = screen.getByRole("checkbox", { name: /model-a/ });
  view.rerender(<NewAPIChannelModelDialog {...props} pending />);
  expect(screen.getByRole("checkbox", { name: /model-a/ })).toBe(checkbox);
  expect(checkbox).toBeChecked();
  expect(checkbox).toHaveAttribute("aria-disabled", "true");
  expect(screen.queryByRole("status", { name: "正在从上游获取模型" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
  view.rerender(<NewAPIChannelModelDialog {...props} error="模型刷新失败" />);
  expect(screen.getByRole("checkbox", { name: /model-a/ })).toBe(checkbox);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.queryByText("模型刷新失败")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
});

it("首次获取模型时显示可见加载提示，取消保持可用且不能确认", () => {
  render(
    <NewAPIChannelModelDialog
      open
      models={[]}
      selected={[]}
      pending
      error=""
      onOpenChange={vi.fn()}
      onSelectedChange={vi.fn()}
      onConfirm={vi.fn()}
    />,
  );
  const status = screen.getByRole("status", { name: "正在从上游获取模型" });
  expect(status).toHaveTextContent("正在从上游获取模型");
  expect(status.querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
});

it("模型读取失败可在弹窗内重试，重试中禁止重复请求", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  const props = {
    open: true,
    models: [],
    selected: [],
    pending: false,
    error: "读取失败",
    onOpenChange: vi.fn(),
    onSelectedChange: vi.fn(),
    onConfirm: vi.fn(),
    onRetry: retry,
  };
  const view = render(<NewAPIChannelModelDialog {...props} />);
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  expect(retry).toHaveBeenCalledOnce();
  view.rerender(<NewAPIChannelModelDialog {...props} pending />);
  expect(screen.getByRole("button", { name: "重新读取" })).toBeDisabled();
});
