import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { AccountSortTableHead } from "@/features/accounts/components/account-sort-header";

describe("AccountSortTableHead", () => {
  it("renders an inactive column as an accessible sort button", () => {
    const markup = renderToStaticMarkup(
      <table>
        <thead>
          <tr>
            <AccountSortTableHead
              label="健康分"
              column="health"
              value="default"
              onValueChange={vi.fn()}
            />
          </tr>
        </thead>
      </table>,
    );

    expect(markup).toContain('aria-sort="none"');
    expect(markup).toContain('aria-label="按健康分升序排列"');
    expect(markup).toContain("健康分");
    expect(markup).toContain('aria-hidden="true"');
  });

  it("exposes the active direction and the next action", () => {
    const markup = renderToStaticMarkup(
      <table>
        <thead>
          <tr>
            <AccountSortTableHead
              label="健康分"
              column="health"
              value="health_asc"
              onValueChange={vi.fn()}
            />
          </tr>
        </thead>
      </table>,
    );

    expect(markup).toContain('aria-sort="ascending"');
    expect(markup).toContain('aria-label="健康分当前升序，切换为降序"');
  });
});
