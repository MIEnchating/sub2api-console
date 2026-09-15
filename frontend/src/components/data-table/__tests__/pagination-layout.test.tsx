import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { DataTablePagination } from "../pagination";

describe("分页布局", () => {
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
