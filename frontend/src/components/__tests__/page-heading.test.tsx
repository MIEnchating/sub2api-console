import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { PageHeading } from "../page-heading";

describe("PageHeading", () => {
  it("renders as the non-scrolling title region", () => {
    const markup = renderToStaticMarkup(
      <PageHeading eyebrow="OPERATIONS" title="运营总览" description="运行状态" />,
    );

    expect(markup).toContain('data-slot="page-heading"');
    expect(markup).toContain("<h1");
    expect(markup).not.toContain("<h2");
    expect(markup).toContain("shrink-0");
    expect(markup).not.toContain("sticky");
    expect(markup).not.toContain("overflow-auto");
  });

  it("keeps title and page actions in one responsive row", () => {
    const markup = renderToStaticMarkup(
      <PageHeading
        eyebrow="OPERATIONS"
        title="账号管理"
        description="账号列表"
        action={<button type="button">同步余额</button>}
      />,
    );

    expect(markup).toContain("flex-nowrap");
    expect(markup).toContain("min-w-0");
    expect(markup).toContain("flex-wrap");
    expect(markup).toContain("shrink-0");
  });
});
