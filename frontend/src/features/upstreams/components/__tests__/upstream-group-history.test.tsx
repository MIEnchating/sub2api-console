import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamGroupHistory } from "../upstream-group-history";

describe("UpstreamGroupHistory", () => {
  it("shows persistent upstream catalog additions and removals", () => {
    const markup = renderToStaticMarkup(
      <UpstreamGroupHistory
        rows={[
          {
            id: 2,
            upstream_id: "up_example",
            group_id: "7",
            group_name: "标准组",
            change_type: "added",
            changed_at: "2026-08-31T01:00:00Z",
          },
          {
            id: 3,
            upstream_id: "up_example",
            group_id: "8",
            group_name: "旧分组",
            change_type: "removed",
            changed_at: "2026-08-31T02:00:00Z",
          },
        ]}
      />,
    );

    expect(markup).toContain("添加");
    expect(markup).toContain("删除");
    expect(markup).toContain("标准组");
    expect(markup).toContain("#7");
    expect(markup).toContain("min-h-0 flex-1 overflow-auto");
    expect(markup).toContain('data-table-panel=""');
  });
});
