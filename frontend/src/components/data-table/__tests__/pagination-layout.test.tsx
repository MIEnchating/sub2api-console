import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { DataTablePagination } from "../pagination";

describe("分页布局", () => {
  it("远端未提供总数时不显示虚构页数和末页入口，仍可继续翻页", async () => {
    const change = vi.fn();
    render(
      <DataTablePagination
        currentPage={2}
        totalPages={3}
        totalItems={-1}
        pageSize={50}
        onPageChange={change}
        onPageSizeChange={vi.fn()}
      />,
    );
    expect(screen.getByText("未提供")).toBeVisible();
    expect(screen.getByLabelText("当前第 2 页")).toBeVisible();
    expect(screen.queryByRole("button", { name: "转到最后一页" })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "转到下一页" }));
    expect(change).toHaveBeenCalledWith(3);
  });

  it("读取远端分页期间禁用容量、页码及翻页，避免重复导航", () => {
    render(
      <DataTablePagination
        currentPage={2}
        totalPages={3}
        totalItems={120}
        pageSize={50}
        disabled
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("combobox", { name: "每页行数" })).toBeDisabled();
    for (const button of screen.getAllByRole("button")) expect(button).toBeDisabled();
  });

  it("多页数据在窄容器中提供可换行的分页和有名称的页容量选择", async () => {
    const onPageChange = vi.fn();
    render(
      <DataTablePagination
        currentPage={50}
        totalPages={100}
        totalItems={1000}
        pageSize={10}
        onPageChange={onPageChange}
        onPageSizeChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("navigation", { name: "表格分页" })).toHaveClass(
      "@container/pagination",
    );
    expect(screen.getByRole("combobox", { name: "每页行数" })).toBeEnabled();
    expect(screen.getByLabelText("当前第 50 页，共 100 页")).toHaveClass("@lg/pagination:hidden");
    await userEvent.setup().click(screen.getByRole("button", { name: "转到下一页" }));
    expect(onPageChange).toHaveBeenCalledWith(51);
  });

  it("空数据分页保留页容量入口且前后翻页不可用", () => {
    render(
      <DataTablePagination
        currentPage={1}
        totalPages={1}
        totalItems={0}
        pageSize={10}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("combobox", { name: "每页行数" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  });
});
