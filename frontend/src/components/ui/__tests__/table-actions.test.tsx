import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../table";

describe("表格固定操作列", () => {
  it("显式启用固定操作列时保持表头语义且键盘可执行行操作", async () => {
    const onEdit = vi.fn();
    const user = userEvent.setup();
    render(
      <Table actionColumn aria-label="账号列表">
        <TableHeader>
          <TableRow>
            <TableHead>账号</TableHead>
            <TableHead>操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>测试账号</TableCell>
            <TableCell overflowTooltip={false}>
              <button type="button" onClick={onEdit}>
                编辑测试账号
              </button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    expect(screen.getByRole("table", { name: "账号列表" })).toHaveAttribute(
      "data-action-column",
      "true",
    );
    expect(screen.getByRole("columnheader", { name: "操作" })).toBeVisible();
    await user.tab();
    expect(screen.getByRole("button", { name: "编辑测试账号" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onEdit).toHaveBeenCalledOnce();
  });

  it.each([undefined, false])("未启用操作列（%s）时不根据最后一列文字自动固定", (actionColumn) => {
    render(
      <Table actionColumn={actionColumn}>
        <TableHeader>
          <TableRow>
            <TableHead>操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>新增账号记录</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    expect(screen.getByRole("table")).not.toHaveAttribute("data-action-column");
  });
});
