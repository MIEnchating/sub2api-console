import { AlignLeft, Copy, Redo2, Search, Undo2 } from "lucide-react";
import type { ReactElement } from "react";
import { EditorAction } from "./editor-action";

export function JsonEditorToolbar(props: {
  disabled?: boolean;
  readOnly?: boolean;
  canUndo: boolean;
  canRedo: boolean;
  canFormat: boolean;
  canCopy: boolean;
  onUndo: () => void;
  onRedo: () => void;
  onFormat: () => void;
  onSearch: () => void;
  onCopy: () => void;
}): ReactElement {
  return (
    <div
      role="group"
      aria-label="编辑器工具"
      className="flex min-w-0 shrink-0 items-center justify-between gap-2 border-b bg-muted/20 px-1.5 py-1"
    >
      {!props.readOnly && (
        <div className="flex shrink-0 items-center">
          <EditorAction
            label="撤销"
            disabled={props.disabled || !props.canUndo}
            onClick={props.onUndo}
          >
            <Undo2 aria-hidden="true" />
          </EditorAction>
          <EditorAction
            label="重做"
            disabled={props.disabled || !props.canRedo}
            onClick={props.onRedo}
          >
            <Redo2 aria-hidden="true" />
          </EditorAction>
        </div>
      )}
      <div className="ml-auto flex shrink-0 items-center">
        {!props.readOnly && (
          <EditorAction
            label="格式化 JSON"
            disabled={props.disabled || !props.canFormat}
            onClick={props.onFormat}
          >
            <AlignLeft aria-hidden="true" />
          </EditorAction>
        )}
        <EditorAction label="查找" disabled={props.disabled} onClick={props.onSearch}>
          <Search aria-hidden="true" />
        </EditorAction>
        <EditorAction
          label="复制内容"
          disabled={props.disabled || !props.canCopy}
          onClick={props.onCopy}
        >
          <Copy aria-hidden="true" />
        </EditorAction>
      </div>
    </div>
  );
}
