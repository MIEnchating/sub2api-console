import { describe, expect, it } from "vitest";

import {
  tooltipArrowStyles,
  tooltipContentStyles,
  tooltipDefaultDelay,
  tooltipDefaultSideOffset,
} from "../tooltip";

describe("Tooltip theme", () => {
  it("uses popover theme tokens instead of inverted foreground colors", () => {
    expect(tooltipContentStyles).toContain("bg-popover");
    expect(tooltipContentStyles).toContain("text-popover-foreground");
    expect(tooltipContentStyles).toContain("border-border");
    expect(tooltipContentStyles).not.toContain("bg-foreground text-background");
    expect(tooltipArrowStyles).toContain("bg-popover");
    expect(tooltipArrowStyles).toContain("fill-popover");
  });

  it("keeps the default popup clear of its trigger", () => {
    expect(tooltipDefaultSideOffset).toBe(8);
  });

  it("opens global tooltips sooner than the library default", () => {
    expect(tooltipDefaultDelay).toBe(300);
  });
});
