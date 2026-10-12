import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { ContentRetry } from "../content-retry";
it("读取失败可用键盘重试，重试进行中禁止重复触发", async () => {
  const retry = vi.fn();
  const view = render(<ContentRetry onRetry={retry} />);
  screen.getByRole("button", { name: "重新读取" }).focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(retry).toHaveBeenCalledOnce();
  view.rerender(<ContentRetry onRetry={retry} pending />);
  expect(screen.getByRole("button", { name: "重新读取" })).toBeDisabled();
});

it("局部重试使用紧凑高度且保留标准按钮尺寸", () => {
  const view = render(<ContentRetry compact onRetry={() => undefined} />);
  expect(view.container.firstElementChild).toHaveClass("min-h-8", "p-0");
  expect(screen.getByRole("button", { name: "重新读取" })).toHaveClass("h-8");
});
