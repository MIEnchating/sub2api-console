import { Popover } from "@base-ui/react/popover";
import { Check, CirclePlus, Search } from "lucide-react";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  dropdownSearchInputClassName,
  focusDropdownSearchOnMount,
} from "@/components/ui/dropdown-search-focus";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export const filterMenuSearchInputClassName = cn(
  "border-input/30 bg-input/30 h-8 rounded-lg pl-8 shadow-none focus-visible:border-input/30",
  dropdownSearchInputClassName,
);

type FilterMenuProps<Value extends string> = {
  label: string;
  options: Value[];
  value: Value | null;
  onValueChange: (value: Value | null) => void;
  optionLabel?: (value: Value) => string;
  clearable?: boolean;
  className?: string;
};

function displayOptionLabel<Value extends string>(
  props: FilterMenuProps<Value>,
  value: Value,
): string {
  return props.optionLabel ? props.optionLabel(value) : value;
}

function optionMatchesSearch(value: string, label: string, query: string): boolean {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  if (!normalizedQuery) return true;
  return `${value} ${label}`.toLocaleLowerCase().includes(normalizedQuery);
}

export function FilterMenu<Value extends string>(props: FilterMenuProps<Value>) {
  const listID = useId();
  const listRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const options = props.options.filter((option) =>
    optionMatchesSearch(option, displayOptionLabel(props, option), query),
  );
  const selectedLabel = props.value !== null ? displayOptionLabel(props, props.value) : null;
  const visibleActiveIndex = Math.min(activeIndex, options.length - 1);
  const activeID = visibleActiveIndex >= 0 ? `${listID}-${visibleActiveIndex}` : undefined;

  useEffect(() => {
    if (!open || !activeID) return;
    listRef.current
      ?.querySelector<HTMLElement>(`[data-active]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [open, activeID, query, props.value]);

  function handleOpenChange(nextOpen: boolean): void {
    setOpen(nextOpen);
    if (!nextOpen) {
      setQuery("");
      setActiveIndex(0);
    }
  }

  function toggleOption(option: Value): void {
    if (props.value === option && props.clearable !== false) {
      props.onValueChange(null);
      return;
    }
    props.onValueChange(option);
  }

  function handleSearchKeyDown(event: KeyboardEvent<HTMLInputElement>): void {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return;
    if (!options.length) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((current) => (Math.min(current, options.length - 1) + 1) % options.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex(
        (current) => (Math.min(current, options.length - 1) - 1 + options.length) % options.length,
      );
    } else if (event.key === "Home") {
      event.preventDefault();
      setActiveIndex(0);
    } else if (event.key === "End") {
      event.preventDefault();
      setActiveIndex(options.length - 1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      toggleOption(options[Math.min(activeIndex, options.length - 1)]);
    }
  }

  return (
    <Popover.Root open={open} onOpenChange={handleOpenChange}>
      <Popover.Trigger
        render={
          <Button
            type="button"
            variant="outline"
            data-press-animation="none"
            className={cn("max-w-64 bg-transparent", props.className)}
            aria-label={`${props.label}筛选`}
          />
        }
      >
        <CirclePlus size={16} aria-hidden="true" />
        <span className="min-w-0 truncate">{props.label}</span>
        {selectedLabel !== null ? (
          <>
            <span className="bg-border mx-1 h-4 w-px shrink-0" aria-hidden="true" />
            <Badge variant="secondary" className="max-w-36 truncate rounded-sm px-1 font-normal">
              {selectedLabel}
            </Badge>
          </>
        ) : null}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner
          side="bottom"
          sideOffset={4}
          align="start"
          collisionPadding={8}
          className="z-50"
        >
          <Popover.Popup
            data-slot="faceted-filter-content"
            aria-label={`${props.label}筛选`}
            initialFocus={false}
            className="bg-popover text-popover-foreground flex max-h-(--available-height) w-[min(22rem,calc(100vw-1rem))] max-w-(--available-width) flex-col overflow-hidden rounded-lg border p-1 shadow-lg outline-none"
          >
            <div className="relative shrink-0 p-1 pb-0">
              <Search
                size={16}
                className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 opacity-60"
                aria-hidden="true"
              />
              <Input
                ref={focusDropdownSearchOnMount}
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setActiveIndex(0);
                }}
                onKeyDown={handleSearchKeyDown}
                placeholder={props.label}
                aria-label={`搜索${props.label}`}
                role="combobox"
                aria-expanded={open}
                aria-autocomplete="list"
                aria-controls={listID}
                aria-activedescendant={activeID}
                className={filterMenuSearchInputClassName}
              />
            </div>
            <div
              ref={listRef}
              id={listID}
              className="min-h-0 max-h-72 overflow-x-hidden overflow-y-auto overscroll-contain p-1"
              role="listbox"
              aria-label={props.label}
            >
              {!options.length ? (
                <div className="text-muted-foreground py-6 text-center text-sm">没有匹配项</div>
              ) : null}
              {options.map((option, index) => (
                <button
                  type="button"
                  data-press-animation="none"
                  role="option"
                  id={`${listID}-${index}`}
                  tabIndex={-1}
                  aria-selected={props.value === option}
                  data-active={visibleActiveIndex === index || undefined}
                  className="hover:bg-muted data-[active]:bg-muted flex min-h-8 w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none"
                  key={option}
                  onMouseEnter={() => setActiveIndex(index)}
                  onClick={() => toggleOption(option)}
                >
                  <span
                    className={cn(
                      "border-primary flex size-4 shrink-0 items-center justify-center rounded-sm border",
                      props.value === option && "bg-primary text-primary-foreground border-primary",
                      props.value !== option && "opacity-50 [&_svg]:invisible",
                    )}
                    aria-hidden="true"
                  >
                    <Check size={14} />
                  </span>
                  <span className="min-w-0 flex-1 truncate">
                    {displayOptionLabel(props, option)}
                  </span>
                </button>
              ))}
            </div>
            {props.value !== null && props.clearable !== false ? (
              <div className="shrink-0 border-t p-1">
                <Button
                  type="button"
                  variant="ghost"
                  data-press-animation="none"
                  className="w-full rounded-sm"
                  onClick={() => props.onValueChange(null)}
                >
                  清除筛选
                </Button>
              </div>
            ) : null}
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
