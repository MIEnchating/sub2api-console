import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { FilterMenu } from "../../App";
import { SearchField } from "../data-table/search-field";

describe("shared toolbar controls", () => {
  it("keeps search and filter controls at the same height", () => {
    const searchMarkup = renderToStaticMarkup(
      <SearchField value="" onChange={() => undefined} placeholder="搜索" />,
    );
    const filterMarkup = renderToStaticMarkup(
      <FilterMenu
        label="分组"
        options={["default"]}
        value={null}
        onValueChange={() => undefined}
      />,
    );

    expect(searchMarkup).toContain("h-8");
    expect(filterMarkup).toContain("h-8");
    expect(filterMarkup).not.toContain("h-7");
  });

  it("matches the faceted-filter trigger without exposing account counts", () => {
    const markup = renderToStaticMarkup(
      <FilterMenu
        label="分组"
        options={["default", "codex"]}
        value="codex"
        onValueChange={() => undefined}
        // @ts-expect-error Account filter options intentionally do not support count suffixes.
        optionCount={() => 12}
      />,
    );

    expect(markup).toContain('aria-label="分组筛选"');
    expect(markup).toContain("border");
    expect(markup).not.toContain("border-dashed");
    expect(markup).toContain('data-press-animation="none"');
    expect(markup).toContain(">分组<");
    expect(markup).toContain(">codex<");
    expect(markup).toContain('data-slot="badge"');
    expect(markup).not.toContain(">12<");
  });
});
