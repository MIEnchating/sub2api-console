import { readdirSync, readFileSync } from "node:fs";
import { dirname, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const sourceRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");

function productionSourceFiles(directory = sourceRoot) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "__tests__" ? [] : productionSourceFiles(path);
    }
    return [".ts", ".tsx"].includes(extname(path)) ? [path] : [];
  });
}

describe("global focus policy", () => {
  it("only allows programmatic automatic focus for dropdown searches", () => {
    const nativeFocusAttribute = ["auto", "Focus"].join("");
    const programmaticFocusCall = new RegExp(`\\.${["fo", "cus"].join("")}\\s*\\(`);
    const enabledInitialFocus = new RegExp(
      `${["initial", "Focus"].join("")}\\s*=\\s*(?!\\{false\\})`,
    );
    const offenders = productionSourceFiles().flatMap((path) => {
      const source = readFileSync(path, "utf8");
      const relativePath = relative(sourceRoot, path);
      const allowsDropdownSearchFocus = relativePath === "components/ui/dropdown-search-focus.ts";
      if (allowsDropdownSearchFocus) return [];
      return source.includes(nativeFocusAttribute) ||
        programmaticFocusCall.test(source) ||
        enabledInitialFocus.test(source)
        ? [relativePath]
        : [];
    });

    expect(offenders).toEqual([]);
  });

  it("explicitly disables the component library initial focus on every popup surface", () => {
    const surfaces = [
      "components/ui/dialog.tsx",
      "components/ui/sheet.tsx",
      "components/ui/combobox.tsx",
      "components/data-table/filter-menu.tsx",
    ];

    for (const path of surfaces) {
      expect(readFileSync(join(sourceRoot, path), "utf8"), path).toContain("initialFocus={false}");
    }
  });

  it("focuses every dropdown search without adding a focus appearance", () => {
    const surfaces = [
      "components/ui/select.tsx",
      "components/ui/combobox.tsx",
      "components/data-table/filter-menu.tsx",
    ];

    for (const path of surfaces) {
      const source = readFileSync(join(sourceRoot, path), "utf8");
      expect(source, path).toContain("ref={focusDropdownSearchOnMount}");
      expect(source, path).toContain("dropdownSearchInputClassName");
    }
  });

  it("uses the shared filter menu instead of faceted selects", () => {
    const offenders = productionSourceFiles().flatMap((path) => {
      const source = readFileSync(path, "utf8");
      return /<Select(?:Trigger|Content|Item)\b[^>]*\bappearance=["']faceted["']/.test(source)
        ? [relative(sourceRoot, path)]
        : [];
    });

    expect(offenders).toEqual([]);
  });
});
