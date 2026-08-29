import { describe, expect, it } from "vitest";

import { schedulingMetric } from "../scheduling-display";

describe("schedulingMetric", () => {
  it("keeps one decimal for fractional allocation values", () => {
    expect(schedulingMetric(57.7)).toBe("57.7");
  });

  it("does not add a decimal to whole allocation values", () => {
    expect(schedulingMetric(58)).toBe("58");
  });
});
