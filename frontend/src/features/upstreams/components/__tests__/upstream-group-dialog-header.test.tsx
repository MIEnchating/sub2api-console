import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { Dialog } from "@/components/ui/dialog";
import { UpstreamGroupDialogHeader } from "../upstream-group-dialog-header";

describe("UpstreamGroupDialogHeader", () => {
  it("reserves the close-button area and keeps the tabs from shrinking into it", () => {
    const markup = renderToStaticMarkup(
      <Dialog open>
        <UpstreamGroupDialogHeader view="history" onViewChange={vi.fn()} />
      </Dialog>,
    );

    expect(markup).toContain("pr-10");
    expect(markup).toContain("shrink-0");
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('data-slot="segmented-control"');
    expect(markup).toContain("变化历史");
  });
});
