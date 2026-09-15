import {
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type KeyboardEvent,
  type ReactElement,
} from "react";
import { runScopeHandlers } from "@codemirror/view";
import {
  closeSearchPanel,
  findNext,
  findPrevious,
  getSearchQuery,
  replaceAll,
  replaceNext,
  SearchQuery,
  selectMatches,
  setSearchQuery,
} from "@codemirror/search";
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  ChevronRight,
  ListChecks,
  Replace,
  ReplaceAll,
  X,
} from "lucide-react";
import { Input } from "@/components/ui/input";
import { EditorAction } from "./editor-action";
import { SearchOptions } from "./search-options";
import type { JsonSearchPanel } from "./search-panel-state";

export function JsonEditorSearchPanel(props: { panel: JsonSearchPanel }): ReactElement {
  const state = useSyncExternalStore(props.panel.subscribe, props.panel.getSnapshot);
  const query = getSearchQuery(state);
  const [expanded, setExpanded] = useState(false);
  const searchInput = useRef<HTMLInputElement>(null);
  const replacementId = useId();
  const statusId = useId();
  const invalid = Boolean(query.search && !query.valid);
  const view = props.panel.view;
  const countQuery = useMemo(
    () =>
      new SearchQuery({
        search: query.search,
        caseSensitive: query.caseSensitive,
        regexp: query.regexp,
        wholeWord: query.wholeWord,
        literal: query.literal,
        test: query.test,
      }),
    [query.search, query.caseSensitive, query.regexp, query.wholeWord, query.literal, query.test],
  );
  const count = useMemo(() => {
    if (!countQuery.valid) return 0;
    const cursor = countQuery.getCursor(view.state);
    let matches = 0;
    // 大文档只统计到展示上限，避免输入时遍历全部匹配。
    while (matches <= 1000 && !cursor.next().done) matches++;
    return matches;
  }, [countQuery, state.doc, view]);
  let status = "";
  if (invalid) status = "正则有误";
  else if (query.valid && !count) status = "无匹配";
  else if (count > 1000) status = "1000+ 处";
  else if (count) status = `${count} 处`;
  const canSearch = query.valid && count > 0;

  useLayoutEffect(() => {
    searchInput.current?.setAttribute("main-field", "true");
    searchInput.current?.focus();
    searchInput.current?.select();
  }, []);

  function updateQuery(changes: Partial<SearchQuery>): void {
    const current = getSearchQuery(view.state);
    view.dispatch({ effects: setSearchQuery.of(new SearchQuery({ ...current, ...changes })) });
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>): void {
    if (event.nativeEvent.isComposing || !props.panel.dom.contains(event.target as Node)) return;
    const isNavigation =
      event.key === "F3" ||
      (event.key.toLowerCase() === "g" && (event.ctrlKey || event.metaKey) && !event.altKey);
    if (!canSearch && isNavigation) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    if (event.key === "Enter" && event.target instanceof HTMLInputElement) {
      event.preventDefault();
      event.stopPropagation();
      if (!canSearch) return;
      if (event.target === searchInput.current) (event.shiftKey ? findPrevious : findNext)(view);
      else if (!view.state.readOnly) replaceNext(view);
      return;
    }
    if (runScopeHandlers(view, event.nativeEvent, "search-panel")) {
      event.preventDefault();
      event.stopPropagation();
    }
  }

  return (
    <div
      role="search"
      aria-label="查找与替换"
      onKeyDown={handleKeyDown}
      className="grid min-w-0 gap-1 p-1.5 font-sans text-sm"
    >
      <div className="json-search-row">
        <div className="relative min-w-0">
          <Input
            ref={searchInput}
            aria-label="查找"
            placeholder="查找"
            value={query.search}
            onChange={(event) => updateQuery({ search: event.target.value })}
            form=""
            autoComplete="off"
            spellCheck={false}
            aria-invalid={invalid}
            aria-describedby={statusId}
            className="pr-20"
          />
          <span
            id={statusId}
            role="status"
            aria-live="polite"
            className="pointer-events-none absolute inset-y-0 right-2 flex items-center text-xs tabular-nums text-muted-foreground"
          >
            {status}
          </span>
        </div>
        <div className="flex shrink-0 items-center justify-end">
          <EditorAction label="上一处" disabled={!canSearch} onClick={() => findPrevious(view)}>
            <ArrowUp aria-hidden="true" />
          </EditorAction>
          <EditorAction label="下一处" disabled={!canSearch} onClick={() => findNext(view)}>
            <ArrowDown aria-hidden="true" />
          </EditorAction>
          <EditorAction
            label="选中全部匹配"
            disabled={!canSearch}
            onClick={() => {
              if (selectMatches(view)) view.focus();
            }}
          >
            <ListChecks aria-hidden="true" />
          </EditorAction>
          {!state.readOnly && (
            <EditorAction
              label={expanded ? "收起替换" : "展开替换"}
              aria-expanded={expanded}
              aria-controls={replacementId}
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}
            </EditorAction>
          )}
          <SearchOptions query={query} onChange={updateQuery} />
          <EditorAction
            label="关闭查找"
            onClick={() => {
              closeSearchPanel(view);
              view.focus();
            }}
          >
            <X aria-hidden="true" />
          </EditorAction>
        </div>
      </div>
      {expanded && !state.readOnly && (
        <div
          id={replacementId}
          className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto_auto] items-center"
        >
          <Input
            aria-label="替换为"
            placeholder="替换为"
            value={query.replace}
            onChange={(event) => updateQuery({ replace: event.target.value })}
            form=""
            autoComplete="off"
            spellCheck={false}
          />
          <EditorAction
            label="替换当前匹配"
            disabled={!canSearch}
            onClick={() => replaceNext(view)}
          >
            <Replace aria-hidden="true" />
          </EditorAction>
          <EditorAction label="全部替换" disabled={!canSearch} onClick={() => replaceAll(view)}>
            <ReplaceAll aria-hidden="true" />
          </EditorAction>
        </div>
      )}
    </div>
  );
}
