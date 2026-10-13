import { useCallback, useId, useMemo, useRef, useState, type ReactElement, type Ref } from "react";
import {
  Combobox,
  ComboboxCollection,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxItem,
  ComboboxList,
  ComboboxSearch,
  ComboboxTrigger,
  useComboboxAnchor,
} from "@/components/ui/combobox";
import { focusDropdownSearchOnMount } from "@/components/ui/dropdown-search-focus";
import { modelID } from "../lib/animation-schema";

const emptyModels: string[] = [];

export function DetectionModelSelect(props: {
  model: string;
  additionalModels?: string[];
  onChange: (model: string, additionalModels: string[]) => void;
  options?: string[];
  disabled: boolean;
  id?: string;
  invalid?: boolean;
  describedBy?: string;
  inputRef?: Ref<HTMLDivElement>;
  onBlur?: () => void;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}): ReactElement {
  const anchor = useComboboxAnchor();
  const popup = useRef<HTMLDivElement>(null);
  const listID = useId();
  const [localOpen, setLocalOpen] = useState(false);
  const [search, setSearch] = useState("");
  const additional = props.additionalModels ?? emptyModels;
  const selected = useMemo(
    () => [props.model, ...additional].filter(Boolean),
    [props.model, additional],
  );
  const query = search.trim();
  const valid = modelID.safeParse(query).success;
  const options = props.options ?? emptyModels;
  const known = useMemo(() => [...new Set([...selected, ...options])], [selected, options]);
  const creating = valid && !known.includes(query);
  const items = creating ? [...known, query] : known;
  const open = !props.disabled && (props.open ?? localOpen);
  const focusSearch = useCallback(
    (element: HTMLInputElement | null): void => {
      if (open) focusDropdownSearchOnMount(element);
    },
    [open],
  );
  const changeOpen = (value: boolean): void => {
    setLocalOpen(value);
    props.onOpenChange?.(value);
    if (!value) setSearch("");
  };
  return (
    <Combobox
      multiple
      modal={false}
      items={items}
      value={selected}
      onValueChange={(values: string[]) => {
        if (values.length > 16) return;
        props.onChange(values[0] ?? "", values.slice(1));
        setSearch("");
      }}
      inputValue={search}
      onInputValueChange={setSearch}
      open={open}
      onOpenChange={(value, details) => {
        // The anchor toggles on click; do not also dismiss it as an outside press.
        const target = details.event.target;
        const nextFocus = "relatedTarget" in details.event ? details.event.relatedTarget : null;
        const clickingAnchor =
          details.reason === "outside-press" &&
          target instanceof Node &&
          anchor.current?.contains(target);
        const focusingAnchor =
          details.reason === "focus-out" &&
          nextFocus instanceof Node &&
          anchor.current?.contains(nextFocus);
        if (!value && (clickingAnchor || focusingAnchor)) {
          details.cancel();
          return;
        }
        changeOpen(value);
      }}
      disabled={props.disabled}
    >
      <ComboboxTrigger
        ref={(element) => {
          anchor.current = element;
          if (typeof props.inputRef === "function") props.inputRef(element);
          else if (props.inputRef) props.inputRef.current = element;
        }}
        id={props.id}
        role="combobox"
        aria-label="检测模型"
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-controls={open ? listID : undefined}
        aria-disabled={props.disabled || undefined}
        aria-invalid={props.invalid}
        aria-describedby={props.describedBy}
        tabIndex={props.disabled ? -1 : 0}
        open={open}
        data-disabled={props.disabled ? "" : undefined}
        className="h-8 flex-nowrap py-0 aria-invalid:border-destructive"
        onBlur={props.onBlur}
        onClick={() => {
          if (!props.disabled) changeOpen(!open);
        }}
        onKeyDown={(event) => {
          if (props.disabled || event.target !== event.currentTarget) return;
          if (["Enter", " ", "ArrowDown"].includes(event.key)) {
            event.preventDefault();
            changeOpen(true);
          }
        }}
      >
        <span className="min-w-0 flex-1 truncate text-left">
          {selected.slice(0, 2).join("、") || "选择模型"}
        </span>
        {selected.length > 2 ? (
          <span
            className="shrink-0 rounded bg-muted px-1.5 text-xs tabular-nums"
            aria-label={`已选 ${selected.length} 个模型`}
          >
            +{selected.length - 2}
          </span>
        ) : null}
      </ComboboxTrigger>
      <ComboboxContent
        ref={popup}
        anchor={anchor}
        className="min-w-[min(20rem,var(--available-width))]"
        finalFocus={() => {
          const active = anchor.current?.ownerDocument.activeElement;
          if (!active || active === document.body || popup.current?.contains(active)) {
            return anchor.current;
          }
          return false;
        }}
      >
        <ComboboxSearch
          ref={focusSearch}
          aria-label="搜索或输入模型 ID"
          placeholder="搜索或输入模型 ID"
          disabled={props.disabled}
        />
        <ComboboxEmpty>输入模型 ID 添加，或先获取模型列表</ComboboxEmpty>
        <ComboboxList id={listID} aria-label="可选检测模型" aria-multiselectable="true">
          <ComboboxCollection>
            {(model: string) => (
              <ComboboxItem
                key={model}
                value={model}
                disabled={selected.length >= 16 && !selected.includes(model)}
              >
                <span className="min-w-0 break-all">
                  {creating && model === query ? `使用模型 ${model}` : model}
                </span>
              </ComboboxItem>
            )}
          </ComboboxCollection>
        </ComboboxList>
        {query && !valid ? (
          <p role="status" className="px-3 py-2 text-xs text-destructive">
            模型 ID 不能超过 256 个字符或包含控制字符
          </p>
        ) : null}
        <div className="flex items-center justify-between border-t px-3 py-2 text-xs text-muted-foreground">
          <span>已选 {selected.length}/16</span>
          <button
            type="button"
            className="hover:text-foreground disabled:opacity-50"
            disabled={props.disabled || selected.length === 0}
            onClick={() => props.onChange("", [])}
          >
            清空模型
          </button>
        </div>
      </ComboboxContent>
    </Combobox>
  );
}
