import { getSearchQuery } from "@codemirror/search";
import type { EditorState } from "@codemirror/state";
import type { EditorView, Panel, ViewUpdate } from "@codemirror/view";

export class JsonSearchPanel implements Panel {
  readonly dom: HTMLElement;
  readonly top: boolean = true;
  private snapshot: EditorState;
  private readonly listeners = new Set<() => void>();

  constructor(
    readonly view: EditorView,
    private readonly onMount: (panel: JsonSearchPanel | null) => void,
  ) {
    this.dom = view.dom.ownerDocument.createElement("div");
    this.dom.className = "cm-json-search";
    this.snapshot = view.state;
  }

  readonly getSnapshot = (): EditorState => this.snapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  mount(): void {
    this.onMount(this);
  }

  update(update: ViewUpdate): void {
    const query = getSearchQuery(update.state);
    const previousQuery = getSearchQuery(update.startState);
    if (
      !update.docChanged &&
      !update.selectionSet &&
      update.state.readOnly === update.startState.readOnly &&
      query.literal === previousQuery.literal &&
      query.eq(previousQuery)
    ) {
      return;
    }
    this.snapshot = update.state;
    this.listeners.forEach((listener) => listener());
  }

  destroy(): void {
    this.listeners.clear();
    this.onMount(null);
  }
}
