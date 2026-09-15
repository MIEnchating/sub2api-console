import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { FieldError } from "../field-error";

it("显式预留错误空间时保持最小两行高度，长错误仍可自然展开", () => {
  const view = render(<FieldError reserveSpace id="test-error" />);
  const slot = view.container.querySelector('[data-slot="field-error"]');
  expect(slot).toHaveClass("min-h-8", "shrink-0", "wrap-anywhere");
  expect(slot).not.toHaveClass("h-8");
  expect(slot).not.toHaveClass("overflow-y-auto");
  expect(slot).toHaveAttribute("aria-hidden", "true");
  expect(slot).not.toHaveAttribute("tabindex");
  const longError = "字段内容不合法，请检查格式后重新提交。".repeat(30);
  view.rerender(<FieldError reserveSpace id="test-error" message={longError} />);
  expect(screen.getByRole("alert")).toHaveTextContent(longError);
  expect(screen.getByRole("alert")).toHaveAttribute("id", "test-error");
  expect(view.container.querySelector('[data-slot="field-error"]')).toBe(slot);
  view.rerender(<FieldError reserveSpace id="test-error" />);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(slot).toBeEmptyDOMElement();
});
