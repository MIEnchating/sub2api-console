import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ResultSummaryRow } from "../result-summary-row";

describe("ResultSummaryRow layout", () => {
  it("keeps labels and long values on one line", () => {
    const markup = renderToStaticMarkup(<ResultSummaryRow label="余额" value="-0.044242" />);

    expect(markup).toContain("justify-between");
    expect(markup).toContain("whitespace-nowrap");
    expect(markup).toContain("truncate");
    expect(markup).not.toContain('title="-0.044242"');
    expect(markup).toContain('data-slot="tooltip-trigger"');
  });

  it("renders an external link when a summary value has a destination", () => {
    const markup = renderToStaticMarkup(
      <ResultSummaryRow label="上游" value="Anc1ent API" href="https://api.anc1ent.top" />,
    );

    expect(markup).toContain('href="https://api.anc1ent.top"');
    expect(markup).toContain('target="_blank"');
    expect(markup).toContain('rel="noreferrer"');
  });
});
