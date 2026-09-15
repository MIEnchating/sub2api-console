import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { ContentLoading } from "../content-loading";

it("局部读取提供可见且可访问的忙碌状态，不展示骨架或虚构进度", () => {
  render(<ContentLoading label="正在读取编辑配置…" />);
  const status = screen.getByRole("status", { name: "正在读取编辑配置…" });
  expect(status).toHaveAttribute("aria-busy", "true");
  expect(screen.getByText("正在读取编辑配置…")).not.toHaveClass("sr-only");
  expect(status.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  expect(status.querySelector("svg")).toHaveClass("motion-safe:animate-spin");
  expect(status.querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});
it("默认局部读取使用紧凑的横向反馈，不为单行文字强制保留 160px 空白", () => {
  render(<ContentLoading label="正在读取预览" />);
  expect(screen.getByRole("status")).toHaveClass("min-h-24", "flex-row");
  expect(screen.getByRole("status")).not.toHaveClass("min-h-40");
});
it("长加载提示可换行，固定高度弹窗可以填满内容区", () => {
  const label = "正在读取" + "很长的配置名称".repeat(30);
  render(<ContentLoading label={label} compact className="h-full" />);
  expect(screen.getByRole("status", { name: label })).toHaveClass("min-w-0", "h-full");
  expect(screen.getByText(label)).toHaveClass("break-words", "max-w-full", "min-w-0");
});
