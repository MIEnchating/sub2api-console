import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { FieldError } from "../field-error";

it("紧凑字段错误浮出时不参与布局，清除后移除提示内容", () => {
  const view = render(<FieldError floating />);
  const slot = view.container.querySelector('[data-slot="field-error"]');
  expect(slot).toHaveClass("h-0", "relative");
  expect(slot).toBeEmptyDOMElement();
  view.rerender(<FieldError floating message="请输入正数" />);
  expect(screen.getByRole("alert")).toHaveClass("absolute", "overflow-y-auto");
  view.rerender(<FieldError floating />);
  expect(slot).toBeEmptyDOMElement();
});

it("错误出现、变长和清除时保持两行空间，超长内容通过滚动完整读取", async () => {
  const user = userEvent.setup();
  const view = render(<FieldError id="test-error" />);
  const slot = view.container.querySelector('[data-slot="field-error"]');
  expect(slot).toHaveClass("h-8", "min-h-8", "shrink-0", "overflow-y-auto", "wrap-anywhere");
  expect(slot).toHaveAttribute("aria-hidden", "true");
  expect(slot).not.toHaveAttribute("tabindex");
  view.rerender(<FieldError id="test-error" message="请输入账号" />);
  await user.tab();
  expect(screen.getByRole("alert")).toHaveFocus();
  const longError = "字段内容不合法，请检查格式后重新提交。".repeat(30);
  view.rerender(<FieldError id="test-error" message={longError} />);
  expect(screen.getByRole("alert")).toHaveTextContent(longError);
  expect(screen.getByRole("alert")).toHaveAttribute("id", "test-error");
  expect(view.container.querySelector('[data-slot="field-error"]')).toBe(slot);
  view.rerender(<FieldError id="test-error" />);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(slot).toBeEmptyDOMElement();
});
