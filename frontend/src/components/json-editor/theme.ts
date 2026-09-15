import { EditorView } from "@codemirror/view";
import { HighlightStyle } from "@codemirror/language";
import { tags } from "@lezer/highlight";

export const editorTheme = EditorView.theme({
  "&": {
    height: "100%",
    color: "var(--foreground)",
    backgroundColor: "var(--background)",
    fontSize: "13px",
  },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": {
    overflow: "auto",
    overscrollBehavior: "contain",
    fontFamily: "var(--font-mono, monospace)",
    lineHeight: "22px",
    scrollbarGutter: "stable",
  },
  ".cm-content": { padding: "8px 0", caretColor: "var(--foreground)" },
  ".cm-line": { padding: "0 12px" },
  ".cm-gutters": {
    backgroundColor: "color-mix(in srgb, var(--muted) 40%, var(--background))",
    color: "var(--muted-foreground)",
    borderRight: "1px solid var(--border)",
    fontSize: "11px",
  },
  ".cm-lineNumbers .cm-gutterElement": { minWidth: "2.5em", padding: "0 4px 0 8px" },
  ".cm-foldGutter .cm-gutterElement": { padding: "0 4px", cursor: "pointer" },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "transparent" },
  "&.cm-focused .cm-activeLine, &.cm-focused .cm-activeLineGutter": {
    backgroundColor: "color-mix(in srgb, var(--primary) 8%, transparent)",
  },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": {
    backgroundColor: "color-mix(in srgb, var(--primary) 25%, transparent)",
  },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--foreground)" },
  ".cm-panels, .cm-tooltip": {
    backgroundColor: "var(--popover)",
    color: "var(--popover-foreground)",
    borderColor: "var(--border)",
  },
  ".cm-json-search": { containerType: "inline-size" },
  ".json-search-row": { display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: "4px" },
  "@container (max-width: 420px)": {
    ".json-search-row": { gridTemplateColumns: "minmax(0, 1fr)" },
  },
  ".cm-textfield": {
    flex: "1 1 10rem",
    minWidth: "min(100%, 10rem)",
    maxWidth: "100%",
    height: "32px",
    borderRadius: "4px",
    padding: "0 7px",
    backgroundColor: "var(--background)",
    color: "var(--foreground)",
    border: "1px solid var(--input)",
  },
  ".cm-button": {
    backgroundImage: "none",
    backgroundColor: "var(--secondary)",
    color: "var(--secondary-foreground)",
    border: "1px solid var(--border)",
    borderRadius: "4px",
    minHeight: "32px",
    padding: "3px 8px",
    whiteSpace: "normal",
    overflowWrap: "anywhere",
  },
  ".cm-diagnostic-error": { borderLeftColor: "var(--destructive)" },
});

export const jsonHighlight = HighlightStyle.define([
  { tag: tags.propertyName, color: "var(--json-property)" },
  { tag: tags.string, color: "var(--success, #16803c)" },
  { tag: tags.number, color: "var(--warning)" },
  { tag: [tags.bool, tags.null], color: "var(--destructive)" },
  { tag: tags.punctuation, color: "var(--muted-foreground)" },
]);
