import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { StatusBadge } from "../status-badge";

describe("StatusBadge", () => {
  it("uses the shared status slot and semantic tone without a filled background", () => {
    const markup = renderToStaticMarkup(<StatusBadge label="开启" variant="success" />);

    expect(markup).toContain('data-slot="status-badge"');
    expect(markup).toContain("text-success");
    expect(markup).not.toContain("bg-success");
  });

  it("converts its compatibility title into the shared tooltip", () => {
    const markup = renderToStaticMarkup(
      <StatusBadge label="待执行" variant="warning" title="等待上一任务完成" />,
    );

    expect(markup).toContain("data-base-ui-tooltip-trigger");
    expect(markup).not.toContain('title="等待上一任务完成"');
  });
});
