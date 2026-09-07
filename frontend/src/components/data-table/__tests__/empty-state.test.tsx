import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Table, TableBody } from "@/components/ui/table";
import { TableEmptyState } from "../empty-state";

describe("TableEmptyState", () => {
  it("spans the data columns while keeping its message in the scrolling viewport", () => {
    render(
      <Table>
        <TableBody>
          <TableEmptyState columns={8}>暂无日志记录</TableEmptyState>
        </TableBody>
      </Table>,
    );
    expect(screen.getByRole("cell")).toHaveAttribute("colspan", "8");
    expect(screen.getByText("暂无日志记录")).toHaveClass("sticky", "left-0", "w-[100cqi]");
    expect(document.querySelector('[data-slot="table-container"]')).toHaveClass("@container/table");
  });

  it("wraps a long empty-state explanation without hiding it in a tooltip", () => {
    const message = "group-".repeat(60);
    render(
      <Table>
        <TableBody>
          <TableEmptyState columns={1}>{message}</TableEmptyState>
        </TableBody>
      </Table>,
    );
    expect(screen.getByText(message)).toHaveClass("[overflow-wrap:anywhere]");
    expect(screen.getByRole("cell")).toHaveClass("whitespace-normal");
    expect(document.querySelector('[data-slot="tooltip-trigger"]')).not.toBeInTheDocument();
  });
});
