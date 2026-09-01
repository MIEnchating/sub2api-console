import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { DataTablePagination, paginationPageSizeSearchable } from "../pagination";

describe("DataTablePagination", () => {
  it("marks the current page and exposes icon button names", () => {
    const markup = renderToStaticMarkup(
      <DataTablePagination
        currentPage={2}
        totalPages={5}
        totalItems={120}
        pageSize={30}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />,
    );

    expect(markup).toContain('aria-current="page"');
    expect(markup).toContain('aria-label="转到上一页"');
    expect(markup).toContain('aria-label="转到下一页"');
    expect(markup).toContain('data-slot="tooltip-trigger"');
    expect(markup).not.toContain("title=");
    expect(markup).toContain("120");
  });

  it("keeps the compact classic select for page size", () => {
    const markup = renderToStaticMarkup(
      <DataTablePagination
        currentPage={1}
        totalPages={2}
        totalItems={12}
        pageSize={10}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />,
    );

    expect(markup).toContain('data-appearance="classic"');
    expect(markup).toContain("lucide-chevron-down");
    expect(markup).not.toContain("lucide-circle-plus");
    expect(markup).not.toContain("border-dashed");
    expect(paginationPageSizeSearchable).toBe(false);
  });
});
