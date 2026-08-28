import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { FilterMenu, SearchField } from "../../App";

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
});
