import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import {
  AnimationAccountsSkeleton,
  AnimationHistorySkeleton,
} from "../animation-accounts-skeleton";

it("自定义检测记录读取时按结果区三列占位，窄屏允许单列显示", () => {
  render(<AnimationHistorySkeleton />);
  const status = screen.getByRole("status", { name: "正在读取自定义检测记录" });
  expect(status).toHaveClass("col-span-full", "grid", "md:grid-cols-3", "min-w-0");
  expect(status).toHaveAttribute("aria-busy", "true");
  for (const card of status.children) expect(card).toHaveClass("min-h-40", "min-w-0");
});

it("首次读取动画账号时按容器自适应网格和未检测卡片紧凑布局占位", () => {
  render(<AnimationAccountsSkeleton />);
  const status = screen.getByRole("status", { name: "正在读取账号" });
  expect(status).toHaveAttribute("aria-busy", "true");
  expect(status).toHaveClass(
    "grid-cols-[repeat(auto-fill,minmax(min(100%,20rem),1fr))]",
    "min-w-0",
  );
  expect(status.children).toHaveLength(4);
  for (const card of status.children) {
    expect(card).toHaveClass("h-auto", "min-w-0", "overflow-hidden");
    expect(card).toHaveAttribute("aria-hidden", "true");
  }
});
