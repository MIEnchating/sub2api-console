import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { SegmentedControl, SegmentedControlItem } from "../segmented-control";

describe("SegmentedControl", () => {
  it("keeps page view switches on one shared compact surface", () => {
    const markup = renderToStaticMarkup(
      <SegmentedControl aria-label="视图">
        <SegmentedControlItem selected>明细</SegmentedControlItem>
        <SegmentedControlItem selected={false}>统计</SegmentedControlItem>
      </SegmentedControl>,
    );

    expect(markup).toContain('data-slot="segmented-control"');
    expect(markup).toContain('data-slot="segmented-control-item"');
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('aria-pressed="false"');
    expect(markup).toContain("rounded-md");
    expect(markup).toContain("bg-muted/40");
  });
});
