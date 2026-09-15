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
