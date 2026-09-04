import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { SegmentedControl, SegmentedControlItem } from "../segmented-control";

describe("SegmentedControl", () => {
  it("keeps page view switches on one shared compact surface", () => {
    const markup = renderToStaticMarkup(
      <SegmentedControl role="tablist" aria-label="视图">
        <SegmentedControlItem role="tab" selected>
          明细
        </SegmentedControlItem>
        <SegmentedControlItem role="tab" selected={false}>
          统计
        </SegmentedControlItem>
      </SegmentedControl>,
    );

    expect(markup).toContain('data-slot="segmented-control"');
    expect(markup).toContain('data-slot="segmented-control-item"');
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('aria-pressed="false"');
    expect(markup).toContain('tabindex="0"');
    expect(markup).toContain('tabindex="-1"');
    expect(markup).toContain("rounded-md");
    expect(markup).toContain("bg-muted/40");
  });
});
