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
