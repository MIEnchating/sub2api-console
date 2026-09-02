import { describe, expect, it, vi } from "vitest";

import { dropdownSearchInputClassName, focusDropdownSearchOnMount } from "../dropdown-search-focus";

describe("dropdown search focus", () => {
  it("defines a shared no-ring appearance for focused dropdown searches", () => {
    expect(dropdownSearchInputClassName).toContain("outline-none");
    expect(dropdownSearchInputClassName).toContain("focus-visible:ring-0");
  });

  it("focuses the search input without scrolling when the dropdown opens", () => {
    const focus = vi.fn();
    const input = { focus } as unknown as HTMLInputElement;

    focusDropdownSearchOnMount(input);

    expect(focus).toHaveBeenCalledWith({ preventScroll: true });
  });

  it("ignores the ref cleanup after the dropdown closes", () => {
    expect(() => focusDropdownSearchOnMount(null)).not.toThrow();
  });
});
