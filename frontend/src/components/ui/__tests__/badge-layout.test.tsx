import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { Badge } from "../badge";

it("长标签保持单行高度并保留可读取的完整名称", () => {
  const label = "long-model-name-".repeat(25);
  render(<Badge>{label}</Badge>);
  expect(screen.getByTitle(label)).toHaveClass("h-5", "max-w-full");
  expect(screen.getByText(label)).toHaveClass("truncate", "min-w-0");
});

it("调用方提供的徽标说明优先于默认完整名称", () => {
  render(<Badge title="请求已完成且通过复核">成功</Badge>);
  expect(screen.getByTitle("请求已完成且通过复核")).toHaveTextContent("成功");
});

it.each([
  ["success", "text-success", "bg-success/10"],
  ["info", "text-info", "bg-info/10"],
  ["warning", "text-warning", "bg-warning/10"],
  ["destructive", "text-destructive", "bg-destructive/10"],
  ["neutral", "text-muted-foreground", "bg-muted/30"],
] as const)("%s 状态标签使用统一语义颜色", (variant, color, background) => {
  render(<Badge variant={variant}>状态</Badge>);
  expect(screen.getByTitle("状态")).toHaveClass(color, background, "h-5", "text-xs");
});
