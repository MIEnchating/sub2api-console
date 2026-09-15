import { applyEdits, format, parseTree, type ParseError } from "jsonc-parser";

export function jsonErrors(value: string): ParseError[] {
  if (!value.trim()) return [];
  const errors: ParseError[] = [];
  parseTree(value, errors, { disallowComments: true, allowTrailingComma: false });
  return errors;
}

export function formatJson(value: string): string {
  if (!value.trim() || jsonErrors(value).length) return value;
  return applyEdits(value, format(value, undefined, { insertSpaces: true, tabSize: 2, eol: "\n" }));
}

export function isJsonDocument(value: string): boolean {
  return !value.trim() || /^\s*[[{]/.test(value) || jsonErrors(value).length === 0;
}
