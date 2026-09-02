import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { calendarPopoverBehavior, DatePicker } from "../date-picker";

describe("DatePicker dismissal", () => {
  it("uses a controlled modal popover so trigger and outside presses close it", () => {
    expect(calendarPopoverBehavior).toEqual({ controlled: true, modal: true });
  });

  it("does not allow a disabled picker to clear its value", () => {
    const markup = renderToStaticMarkup(
      createElement(DatePicker, {
        selected: new Date(2026, 7, 31),
        onSelect: () => undefined,
        label: "核算日期",
        disabled: true,
      }),
    );
    const clearButton = markup.match(/<button[^>]*aria-label="清除日期"[^>]*>/)?.[0];

    expect(clearButton).toContain('disabled=""');
  });
});
