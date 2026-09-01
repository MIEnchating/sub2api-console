import { describe, expect, it } from "vitest";

import { getClientPage } from "../use-client-pagination";

describe("getClientPage", () => {
  const items = Array.from({ length: 25 }, (_, index) => index + 1);

  it("returns the requested page and total page count", () => {
    expect(getClientPage(items, 2, 10)).toEqual({
      currentPage: 2,
      totalPages: 3,
      visibleItems: [11, 12, 13, 14, 15, 16, 17, 18, 19, 20],
    });
  });

  it("clamps a stale page after the item count shrinks", () => {
    expect(getClientPage(items, 99, 10)).toEqual({
      currentPage: 3,
      totalPages: 3,
      visibleItems: [21, 22, 23, 24, 25],
    });
  });

  it("keeps an empty collection on page one", () => {
    expect(getClientPage([], 4, 20)).toEqual({
      currentPage: 1,
      totalPages: 1,
      visibleItems: [],
    });
  });
});
