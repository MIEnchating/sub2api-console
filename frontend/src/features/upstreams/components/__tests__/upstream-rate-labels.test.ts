import { describe, expect, it } from "vitest";

import { upstreamRateLabels } from "../../lib/upstream-rate-labels";

describe("upstream rate terminology", () => {
  it("uses neutral labels for Console-side converted values", () => {
    expect(upstreamRateLabels.mappingFormula).toContain("换算后");
    expect(upstreamRateLabels.mappedBalance).toBe("换算后余额");
    expect(upstreamRateLabels.effectiveRate).toBe("换算后倍率");
    expect(Object.values(upstreamRateLabels).join(" ")).not.toContain("下游");
  });
});
