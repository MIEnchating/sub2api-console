import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AccountHealthScore } from "../account-health-score";

describe("AccountHealthScore", () => {
  it("shows the shared score ring and short-term and long-term scores", () => {
    const markup = renderToStaticMarkup(
      <AccountHealthScore score={72.5} shortScore={68} longScore={83} sampleCount={2} />,
    );

    expect(markup).toContain('data-slot="account-health-score"');
    expect(markup).toContain("健康分 72.5");
    expect(markup).toContain("stroke-warning");
    expect(markup).toContain("短期 68");
    expect(markup).toContain("长期 83");
  });

  it("shows no score when there are no valid samples", () => {
    const markup = renderToStaticMarkup(
      <AccountHealthScore score={72.5} shortScore={68} longScore={83} sampleCount={0} />,
    );

    expect(markup).toContain("暂无健康分");
    expect(markup).toContain("短期 —");
    expect(markup).toContain("长期 —");
    expect(markup).not.toContain("健康分 72.5");
  });
});
