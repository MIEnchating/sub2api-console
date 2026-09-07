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
  expect(screen.getByRole("alert")).toHaveTextContent("模型刷新失败");
  expect(screen.getByRole("button", { name: "确认模型" })).toBeDisabled();
});
