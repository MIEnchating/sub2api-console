import { describe, expect, it } from "vitest";

import { calendarPopoverBehavior } from "../date-picker";

describe("DatePicker dismissal", () => {
  it("uses a controlled modal popover so trigger and outside presses close it", () => {
    expect(calendarPopoverBehavior).toEqual({ controlled: true, modal: true });
  });
});
