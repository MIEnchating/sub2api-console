import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { PageHeading } from "../page-heading";
import { PageLayout } from "../page-layout";

describe("PageLayout", () => {
  it("keeps the heading outside the single scrolling content region", () => {
    const markup = renderToStaticMarkup(
      <PageLayout>
        <PageHeading
          eyebrow="OPERATIONS"
          title="账号管理"
          description="账号列表"
          action={<button type="button">刷新</button>}
        />
        <section>账号表格</section>
      </PageLayout>,
    );

    const headingIndex = markup.indexOf('data-slot="page-heading"');
    const contentIndex = markup.indexOf('data-slot="page-content"');

    expect(markup).toContain('data-slot="page-layout"');
    expect(markup).not.toContain("<main");
    expect(headingIndex).toBeGreaterThan(-1);
    expect(contentIndex).toBeGreaterThan(headingIndex);
    expect(markup).toContain("min-h-0 flex-1 overflow-auto");
    expect(markup).not.toContain('data-slot="page-footer"');
  });

  it("supports a fixed content slot without adding another page scrollbar", () => {
    const markup = renderToStaticMarkup(
      <PageLayout fixedContent>
        <PageHeading eyebrow="OPERATIONS" title="账号管理" description="账号列表" />
        <section>账号表格</section>
      </PageLayout>,
    );

    expect(markup).toContain("min-h-0 flex-1 overflow-hidden");
    expect(markup).not.toContain("min-h-0 flex-1 overflow-auto");
  });
});
