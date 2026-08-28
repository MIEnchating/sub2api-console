import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { DataTablePagination } from "../pagination";

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
});
