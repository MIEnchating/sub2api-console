import { describe, expect, it } from "vitest";

import {
  pruneAccountSelection,
  sameAccountSelection,
  updateAccountSelection,
} from "../account-selection";

describe("account selection", () => {
  it("selects the current page without discarding accounts selected on another page", () => {
    const selected = updateAccountSelection(new Set(["1"]), ["51", "52"], true);

    expect([...selected]).toEqual(["1", "51", "52"]);
  });

  it("clears only the current page when the page selection is removed", () => {
    const selected = updateAccountSelection(new Set(["1", "51", "52"]), ["51", "52"], false);

    expect([...selected]).toEqual(["1"]);
  });

  it("removes selected accounts that no longer exist after a server refresh", () => {
    const selected = pruneAccountSelection(new Set(["1", "2", "3"]), ["1", "3", "4"]);

    expect([...selected]).toEqual(["1", "3"]);
  });

  it("compares selections independently of insertion order", () => {
    expect(sameAccountSelection(new Set(["1", "2"]), new Set(["2", "1"]))).toBe(true);
  });
});
