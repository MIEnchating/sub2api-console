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
  it("does not allow native or programmatic automatic focus", () => {
    const nativeFocusAttribute = ["auto", "Focus"].join("");
    const programmaticFocusCall = new RegExp(`\\.${["fo", "cus"].join("")}\\s*\\(`);
    const enabledInitialFocus = new RegExp(
      `${["initial", "Focus"].join("")}\\s*=\\s*(?!\\{false\\})`,
    );
    const offenders = productionSourceFiles().flatMap((path) => {
      const source = readFileSync(path, "utf8");
      return source.includes(nativeFocusAttribute) ||
        programmaticFocusCall.test(source) ||
        enabledInitialFocus.test(source)
        ? [relative(sourceRoot, path)]
        : [];
    });

    expect(offenders).toEqual([]);
  });

  it("explicitly disables the component library initial focus on every popup surface", () => {
    const surfaces = [
      "components/ui/dialog.tsx",
      "components/ui/sheet.tsx",
      "components/ui/combobox.tsx",
      "App.tsx",
    ];

    for (const path of surfaces) {
      expect(readFileSync(join(sourceRoot, path), "utf8"), path).toContain("initialFocus={false}");
    }
  });
});
