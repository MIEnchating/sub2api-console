import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const sourceRoot = join(import.meta.dirname, "../../..");
const titleForwardingComponents = new Set([
  "Badge",
  "Button",
  "Checkbox",
  "Input",
  "Link",
  "SelectTrigger",
  "Switch",
  "Textarea",
]);

function sourceFiles(directory) {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sourceFiles(path);
    return path.endsWith(".tsx") && !path.includes(`${join("__tests__", "")}`) ? [path] : [];
  });
}

function forbiddenTitleAttributes(file) {
  const source = ts.createSourceFile(
    file,
    readFileSync(file, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const findings = [];
  function visit(node) {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = node.tagName.getText(source);
      const forwardsToNativeElement = /^[a-z]/.test(tag) || titleForwardingComponents.has(tag);
      const hasTitle = node.attributes.properties.some(
        (attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText(source) === "title",
      );
      if (forwardsToNativeElement && hasTitle) {
        const location = source.getLineAndCharacterOfPosition(node.getStart(source));
        findings.push(`${relative(sourceRoot, file)}:${location.line + 1} <${tag}>`);
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  return findings;
}

describe("native browser title attributes", () => {
  it("uses the shared Tooltip component instead of native title popups", () => {
    expect(sourceFiles(sourceRoot).flatMap(forbiddenTitleAttributes)).toEqual([]);
  });
});
