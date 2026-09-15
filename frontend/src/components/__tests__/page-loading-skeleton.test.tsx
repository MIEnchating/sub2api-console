import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { PageLoadingSkeleton } from "../page-loading-skeleton";

it("表单首次读取时占位输入与正式控件同为 32px，字段标签独立排列", () => {
  render(<PageLoadingSkeleton label="正在读取设置" variant="form" />);
  const controls = screen.getByRole("status").querySelectorAll('[data-slot="skeleton-control"]');
  expect(controls).toHaveLength(4);
  for (const control of controls) expect(control).toHaveClass("h-8", "w-full");
});

it("单个表单加载时占据整行，不预留第二张表单的位置", () => {
  render(<PageLoadingSkeleton label="正在读取设置" variant="form" panels={1} />);
  const grid = screen.getByRole("status").querySelector('[aria-hidden="true"]');
  expect(grid).not.toHaveClass("lg:grid-cols-2");
  expect(grid?.children).toHaveLength(1);
});

it("明确指定双表单的页面才使用两列骨架", () => {
  render(<PageLoadingSkeleton label="正在读取双表单设置" variant="form" panels={2} />);
  const grid = screen.getByRole("status").querySelector('[aria-hidden="true"]');
  expect(grid).toHaveClass("lg:grid-cols-2");
  expect(grid?.children).toHaveLength(2);
});

it("固定表格工作区加载时保留表头和底部分页，行溢出限制在内容区", () => {
  render(<PageLoadingSkeleton label="正在读取表格" fill />);
  const loading = screen.getByRole("status");
  expect(loading).toHaveClass("h-full", "flex", "min-h-0");
  expect(loading.querySelector('[data-slot="skeleton-table-header"]')).toHaveClass(
    "h-10",
    "shrink-0",
  );
  expect(loading.querySelector('[data-slot="skeleton-rows"]')).toHaveClass(
    "min-h-0",
    "overflow-hidden",
  );
  expect(loading.querySelector('[data-slot="skeleton-pagination"]')).toHaveClass(
    "shrink-0",
    "mt-auto",
  );
});

it("嵌入已有面板的列表加载不再重复描边，也不显示表格分页占位", () => {
  render(<PageLoadingSkeleton label="正在读取列表" variant="list" framed={false} />);
  const loading = screen.getByRole("status");
  expect(loading.querySelector(".border")).toBeNull();
  expect(loading.querySelector('[data-slot="skeleton-pagination"]')).toBeNull();
});

it.each(["table", "form", "list"] as const)("%s 骨架外框沿用内容面板的统一圆角", (variant) => {
  render(<PageLoadingSkeleton label="正在读取页面" variant={variant} />);
  const frames = screen.getByRole("status").querySelectorAll(".border");
  expect(frames.length).toBeGreaterThan(0);
  for (const frame of frames) expect(frame).toHaveClass("rounded-lg");
});

it.each(["table", "form", "list"] as const)(
  "%s 骨架提供加载状态且装饰内容不进入可访问树",
  (variant) => {
    render(<PageLoadingSkeleton label="正在读取页面" variant={variant} />);
    const loading = screen.getByRole("status", { name: "正在读取页面" });
    expect(loading).toHaveAttribute("aria-busy", "true");
    expect(loading).toHaveClass("min-w-0", "w-full");
    for (const placeholder of loading.querySelectorAll('[data-slot="skeleton"]')) {
      expect(placeholder.closest('[aria-hidden="true"]')).not.toBeNull();
    }
    expect(loading.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(1);
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  },
);
