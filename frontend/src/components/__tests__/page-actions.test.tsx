import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { PageActions } from "../page-actions";

describe("PageActions", () => {
  it("keeps page-level actions aligned and allows them to wrap", () => {
    const markup = renderToStaticMarkup(
      <PageActions>
        <button type="button">同步余额</button>
        <button type="button">名称修复</button>
      </PageActions>,
    );

    expect(markup).toContain('data-slot="page-actions"');
    expect(markup).toContain("flex-wrap");
    expect(markup).toContain("items-center");
    expect(markup).toContain("justify-end");
    expect(markup).toContain("gap-2");
    expect(markup).toContain("同步余额");
    expect(markup).toContain("名称修复");
  });

  it("accepts page-specific layout classes without replacing the shared contract", () => {
    const markup = renderToStaticMarkup(
      <PageActions className="w-full">
        <button type="button">操作</button>
      </PageActions>,
    );

    expect(markup).toContain("w-full");
    expect(markup).toContain("flex-wrap");
    expect(markup).toContain("justify-end");
  });
});
