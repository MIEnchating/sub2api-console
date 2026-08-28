import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { PageHeading } from "../page-heading";

describe("PageHeading", () => {
  it("renders as the non-scrolling title region", () => {
    const markup = renderToStaticMarkup(
      <PageHeading eyebrow="OPERATIONS" title="运营总览" description="运行状态" />,
    );

    expect(markup).toContain('data-slot="page-heading"');
    expect(markup).toContain("shrink-0");
    expect(markup).not.toContain("sticky");
    expect(markup).not.toContain("overflow-auto");
  });
});
