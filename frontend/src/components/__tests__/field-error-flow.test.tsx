import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { FieldError } from "../field-error";

it("没有错误时不占用表单空间，清除错误后移除提示", () => {
  const view = render(<FieldError />);
  expect(view.container).toBeEmptyDOMElement();
  view.rerender(<FieldError message="请输入账号" />);
  expect(screen.getByRole("alert")).toHaveTextContent("请输入账号");
  view.rerender(<FieldError />);
  expect(view.container).toBeEmptyDOMElement();
});

it("长错误在文档流中换行展示，不固定高度或覆盖后续字段", () => {
  const message = "字段内容不合法，请检查格式后重新提交。".repeat(30);
  render(<FieldError id="test-error" message={message} />);
  const alert = screen.getByRole("alert");
  expect(alert).toHaveTextContent(message);
  expect(alert).toHaveAttribute("id", "test-error");
  expect(alert).toHaveClass("block", "min-w-0", "wrap-anywhere");
  expect(alert).not.toHaveClass("h-8");
  expect(alert).not.toHaveClass("absolute");
  expect(alert).not.toHaveClass("overflow-y-auto");
  expect(alert).not.toHaveAttribute("tabindex");
});
