import { useEffect, useRef, useState, type ReactElement } from "react";
import { createPortal } from "react-dom";
import { basicSetup } from "codemirror";
import { Annotation, Compartment, EditorState } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { json } from "@codemirror/lang-json";
import { syntaxHighlighting } from "@codemirror/language";
import { undo, redo, undoDepth, redoDepth } from "@codemirror/commands";
import { closeSearchPanel, openSearchPanel, search } from "@codemirror/search";
import { linter } from "@codemirror/lint";
import { Braces, LockKeyhole } from "lucide-react";
import { toast } from "sonner";
import { JsonEditorToolbar } from "./toolbar";
import { formatJson, isJsonDocument, jsonErrors } from "./json-document";
import { editorTheme, jsonHighlight } from "./theme";
import type { JsonEditorProps } from "./types";
import { JsonSearchPanel } from "./search-panel-state";
import { JsonEditorSearchPanel } from "./search-panel";

const phrases = {
  Find: "查找",
  Replace: "替换",
  next: "下一处",
  previous: "上一处",
  all: "全部",
  "match case": "区分大小写",
  regexp: "正则表达式",
  "by word": "全字匹配",
  replace: "替换",
  "replace all": "全部替换",
  close: "关闭",
  "Fold line": "折叠此行",
  "Unfold line": "展开此行",
  to: "至",
};
const externalUpdate = Annotation.define<boolean>();

function options(props: JsonEditorProps, invalid = false) {
  const editable = !props.disabled && !props.readOnly;
  return [
    EditorState.readOnly.of(!editable),
    EditorView.editable.of(editable),
    EditorView.contentAttributes.of({
      id: props.id ?? "",
      "aria-label": props["aria-label"],
      role: "textbox",
      "aria-multiline": "true",
      "aria-describedby": props["aria-describedby"] ?? "",
      "aria-invalid": String(Boolean(props["aria-invalid"] || invalid)),
      "aria-readonly": String(Boolean(props.readOnly)),
      "aria-disabled": String(Boolean(props.disabled)),
      tabindex: props.disabled ? "-1" : "0",
    }),
    placeholder(props.placeholder ?? ""),
  ];
}

export default function JsonEditorContent(props: JsonEditorProps): ReactElement {
  const mount = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const latest = useRef(props);
  const configuration = useRef(new Compartment());
  const language = useRef(new Compartment());
  const [cursor, setCursor] = useState("1:1");
  const [copying, setCopying] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [history, setHistory] = useState({ undo: false, redo: false });
  const [searchPanel, setSearchPanel] = useState<JsonSearchPanel | null>(null);
  latest.current = props;
  const useJson = props.language !== "auto" || isJsonDocument(props.value);

  useEffect(() => {
    if (!mount.current) return;
    const editor = new EditorView({
      parent: mount.current,
      doc: latest.current.value,
      extensions: [
        basicSetup,
        search({
          top: true,
          createPanel: (instance) => new JsonSearchPanel(instance, setSearchPanel),
        }),
        editorTheme,
        syntaxHighlighting(jsonHighlight),
        EditorView.lineWrapping,
        EditorState.phrases.of(phrases),
        configuration.current.of(options(latest.current)),
        language.current.of(
          latest.current.language !== "auto" || isJsonDocument(latest.current.value) ? json() : [],
        ),
        linter((instance) => {
          const text = instance.state.doc.toString();
          const errors =
            latest.current.language === "auto" && !isJsonDocument(text) ? [] : jsonErrors(text);
          setInvalid(errors.length > 0);
          return errors.map((error) => ({
            from: error.offset,
            to: Math.min(instance.state.doc.length, error.offset + error.length),
            severity: "error" as const,
            message: "JSON 语法错误，请检查引号、逗号与括号",
          }));
        }),
        // Tab 保持常规表单焦点导航，缩进使用 CodeMirror 的默认快捷键。
        keymap.of([
          {
            key: "Mod-Shift-f",
            run: (instance) => {
              if (instance.state.readOnly) return false;
              const formatted = formatJson(instance.state.doc.toString());
              instance.dispatch({
                changes: { from: 0, to: instance.state.doc.length, insert: formatted },
              });
              return true;
            },
          },
        ]),
        EditorView.updateListener.of((update) => {
          if (
            update.docChanged &&
            !update.transactions.some((transaction) => transaction.annotation(externalUpdate))
          )
            latest.current.onChange?.(update.state.doc.toString());
          if (update.docChanged || update.selectionSet) {
            const head = update.state.selection.main.head;
            const line = update.state.doc.lineAt(head);
            setCursor(`${line.number}:${head - line.from + 1}`);
            setHistory({ undo: undoDepth(update.state) > 0, redo: redoDepth(update.state) > 0 });
          }
        }),
        EditorView.domEventHandlers({ blur: () => latest.current.onBlur?.() }),
      ],
    });
    view.current = editor;
    return () => {
      editor.destroy();
      view.current = null;
    };
    // Editor 生命周期独立于表单更新，后续变化通过 compartment 和 transaction 同步。
  }, []);

  useEffect(() => {
    const content = view.current?.contentDOM;
    const element = content instanceof HTMLDivElement ? content : null;
    const ref = props.inputRef;
    if (typeof ref === "function") {
      const cleanup = ref(element);
      return () => {
        if (cleanup) cleanup();
        else ref(null);
      };
    }
    if (ref) ref.current = element;
    return () => {
      if (ref) ref.current = null;
    };
  }, [props.inputRef]);

  useEffect(() => {
    const editor = view.current;
    if (editor && editor.state.doc.toString() !== props.value) {
      editor.dispatch({
        changes: { from: 0, to: editor.state.doc.length, insert: props.value },
        annotations: externalUpdate.of(true),
      });
    }
  }, [props.value]);

  useEffect(() => {
    if (props.disabled && view.current) closeSearchPanel(view.current);
    view.current?.dispatch({ effects: configuration.current.reconfigure(options(props, invalid)) });
  }, [
    props.disabled,
    props.readOnly,
    props.id,
    props.placeholder,
    props["aria-label"],
    props["aria-describedby"],
    props["aria-invalid"],
    invalid,
  ]);

  useEffect(() => {
    view.current?.dispatch({ effects: language.current.reconfigure(useJson ? json() : []) });
  }, [useJson]);

  function format(): void {
    const editor = view.current;
    if (!editor) return;
    if (jsonErrors(props.value).length) {
      toast.error("JSON 格式有误，请修正后再格式化");
      return;
    }
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: formatJson(props.value) },
    });
    editor.focus();
  }

  async function copy(): Promise<void> {
    setCopying(true);
    try {
      await navigator.clipboard.writeText(props.value);
      toast.success("已复制");
    } catch {
      toast.error("复制失败，请允许剪贴板权限后重试");
    } finally {
      setCopying(false);
    }
  }

  return (
    <>
      <JsonEditorToolbar
        disabled={props.disabled}
        readOnly={props.readOnly}
        canUndo={history.undo}
        canRedo={history.redo}
        canFormat={Boolean(props.value.trim()) && useJson}
        canCopy={!copying && Boolean(props.value)}
        onUndo={() => {
          if (view.current) {
            undo(view.current);
            view.current.focus();
          }
        }}
        onRedo={() => {
          if (view.current) {
            redo(view.current);
            view.current.focus();
          }
        }}
        onFormat={format}
        onSearch={() => {
          if (view.current) openSearchPanel(view.current);
        }}
        onCopy={() => void copy()}
      />
      <div ref={mount} className="min-h-0 min-w-0 flex-1 overflow-hidden" />
      {searchPanel && createPortal(<JsonEditorSearchPanel panel={searchPanel} />, searchPanel.dom)}
      <div className="flex h-6 min-w-0 shrink-0 items-center justify-between gap-3 border-t bg-muted/20 px-3 text-xs text-muted-foreground">
        <span className="flex shrink-0 items-center gap-1.5">
          <Braces className="size-3" aria-hidden="true" />
          {useJson ? "JSON" : "文本"}
          {props.readOnly && (
            <span className="ml-1 flex items-center gap-1">
              <LockKeyhole className="size-3" aria-hidden="true" />
              只读
            </span>
          )}
        </span>
        <span aria-label="光标位置" className="min-w-0 font-mono tabular-nums">
          {cursor}
        </span>
      </div>
    </>
  );
}
