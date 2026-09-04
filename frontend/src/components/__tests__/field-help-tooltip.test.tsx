import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { tooltipContentStyles } from "../ui/tooltip";
import { cn } from "../../lib/utils";
import {
  fieldHelpTooltipContentStyles,
  fieldHelpTooltipPosition,
  FieldHelpTooltip,
  FieldLabel,
} from "../field-help-tooltip";

describe("field help tooltip", () => {
  it("uses a keyboard-focusable question icon with an accessible name", () => {
    const markup = renderToStaticMarkup(
      <FieldHelpTooltip label="请求超时">允许 1 至 120 秒</FieldHelpTooltip>,
    );

    expect(markup).toContain('<button type="button"');
    expect(markup).toContain('aria-label="请求超时说明"');
    expect(markup).toContain('data-slot="tooltip-trigger"');
    expect(markup).toContain('aria-hidden="true"');
  });

  it("places selectable help beside its trigger so moving into it does not cross an adjacent row", () => {
    expect(fieldHelpTooltipPosition).toEqual({ side: "inline-end", align: "start" });
    expect(fieldHelpTooltipContentStyles).toContain("pointer-events-auto");
    expect(fieldHelpTooltipContentStyles).toContain("select-text");
  });

  it("uses normal text flow so rich descriptions do not wrap as separate flex items", () => {
    const resolvedStyles = cn(tooltipContentStyles, fieldHelpTooltipContentStyles);

    expect(resolvedStyles).toContain("block");
    expect(resolvedStyles).toContain("whitespace-normal");
    expect(resolvedStyles).toContain("break-words");
    expect(resolvedStyles).not.toContain("inline-flex");
  });

  it("associates a field label with its input without nesting the help button in the label", () => {
    const markup = renderToStaticMarkup(
      <FieldLabel label="请求超时" description="允许 1 至 120 秒" htmlFor="timeout" />,
    );

    expect(markup).toContain('<label for="timeout">请求超时</label><button');
    expect(markup).not.toContain('<label for="timeout"><button');
  });
});
