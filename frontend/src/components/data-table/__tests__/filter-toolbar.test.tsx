import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TableFilterToolbar } from "../filter-toolbar";

describe("TableFilterToolbar", () => {
  it("renders an unframed toolbar for placement above a table card", () => {
    const markup = renderToStaticMarkup(
      <TableFilterToolbar>
        <input aria-label="搜索" />
      </TableFilterToolbar>,
    );

    expect(markup).toContain('data-slot="table-filter-toolbar"');
    expect(markup).toContain('aria-label="搜索"');
    expect(markup).not.toContain("border-b");
    expect(markup).not.toContain("px-3");
  });
});
