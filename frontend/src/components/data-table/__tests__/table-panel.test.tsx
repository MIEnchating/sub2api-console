import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { DataTablePanel } from "../table-panel";

describe("DataTablePanel", () => {
  it("uses the shared card surface for every scrollable data region", () => {
    const markup = renderToStaticMarkup(
      <DataTablePanel className="flex-1">
        <table aria-label="数据" />
      </DataTablePanel>,
    );

    expect(markup).toContain('data-table-panel=""');
    expect(markup).toContain('data-slot="card"');
    expect(markup).toContain("rounded-[8px]");
    expect(markup).toContain("ring-1");
    expect(markup).toContain("min-h-0");
    expect(markup).toContain("overflow-hidden");
    expect(markup).not.toContain("border-dashed");
  });
});
