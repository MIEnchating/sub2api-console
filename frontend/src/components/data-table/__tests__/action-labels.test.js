import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const sourceRoot = join(import.meta.dirname, "../../..");

function sourceFiles(directory) {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sourceFiles(path);
    return path.endsWith(".tsx") && !path.includes(`${join("__tests__", "")}`) ? [path] : [];
  });
}

function isActionOnlyLabel(expression) {
  if (ts.isStringLiteral(expression) || ts.isNoSubstitutionTemplateLiteral(expression)) return true;
  if (
    ts.isPropertyAccessExpression(expression) &&
    ts.isIdentifier(expression.expression) &&
    expression.expression.text === "accountOperationCopy"
  ) {
    return true;
  }
  if (ts.isParenthesizedExpression(expression)) return isActionOnlyLabel(expression.expression);
  if (ts.isConditionalExpression(expression)) {
    return isActionOnlyLabel(expression.whenTrue) && isActionOnlyLabel(expression.whenFalse);
  }
  return false;
}

function dynamicActionLabels(file) {
  const source = ts.createSourceFile(
    file,
    readFileSync(file, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const findings = [];
  function visit(node) {
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      node.tagName.getText(source) === "TableActionButton"
    ) {
      const label = node.attributes.properties.find(
        (attribute) => ts.isJsxAttribute(attribute) && attribute.name.getText(source) === "label",
      );
      const valid =
        label &&
        ts.isJsxAttribute(label) &&
        (ts.isStringLiteral(label.initializer) ||
          (ts.isJsxExpression(label.initializer) &&
            label.initializer.expression &&
            isActionOnlyLabel(label.initializer.expression)));
      if (!valid) {
        const location = source.getLineAndCharacterOfPosition(node.getStart(source));
        findings.push(`${relative(sourceRoot, file)}:${location.line + 1}`);
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  return findings;
}

describe("table action labels", () => {
  it("keeps tooltips action-only instead of appending row object names", () => {
    expect(sourceFiles(sourceRoot).flatMap(dynamicActionLabels)).toEqual([]);
  });
});
