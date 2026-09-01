/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import * as React from "react";

import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxCollection,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxItem,
  ComboboxList,
  ComboboxSearch,
  ComboboxTrigger,
  ComboboxValue,
  useComboboxAnchor,
} from "@/components/ui/combobox";
import { cn } from "@/lib/utils";

export type MultiSelectOption = {
  label: string;
  value: string;
};

type MultiSelectProps = {
  options: MultiSelectOption[];
  selected: string[];
  onChange: (values: string[]) => void;
  title?: string;
  searchPlaceholder?: string;
  emptyText?: string;
  clearText?: string;
  unknownValueLabel?: string;
  className?: string;
  ariaLabel?: string;
  disabled?: boolean;
  singleSelect?: boolean;
  showTitle?: boolean;
  maxVisibleChips?: number;
  renderOption?: (option: MultiSelectOption) => React.ReactNode;
};

export function nextMultiSelectValues(
  selected: string[],
  value: string,
  singleSelect = false,
): string[] {
  if (singleSelect) {
    return selected.includes(value) ? [] : [value];
  }
  if (selected.includes(value)) {
    return selected.filter((item) => item !== value);
  }
  return [...selected, value];
}

export function matchesMultiSelectOption(option: MultiSelectOption, search: string): boolean {
  const query = search.trim().toLocaleLowerCase();
  if (!query) return true;
  return `${option.value} ${option.label}`.toLocaleLowerCase().includes(query);
}

export function shouldCloseMultiSelectFromTriggerPress(
  open: boolean,
  button: number,
  removeButton: boolean,
): boolean {
  return open && button === 0 && !removeButton;
}

export function MultiSelectClearAction(props: {
  text: string;
  disabled: boolean;
  onClear: () => void;
}) {
  return (
    <div className="border-border border-t p-1">
      <button
        type="button"
        className="hover:bg-accent hover:text-accent-foreground flex w-full items-center rounded-md px-2 py-1.5 text-left text-sm disabled:pointer-events-none disabled:opacity-50"
        disabled={props.disabled}
        onClick={props.onClear}
      >
        {props.text}
      </button>
    </div>
  );
}

export function MultiSelect(props: MultiSelectProps) {
  const triggerAnchorRef = useComboboxAnchor();
  const [open, setOpen] = React.useState(false);
  const [inputValue, setInputValue] = React.useState("");
  const [expanded, setExpanded] = React.useState(false);
  const closingFromTriggerPress = React.useRef(false);
  const title = props.title ?? "选择选项";
  const showTitle = props.showTitle ?? true;
  const searchPlaceholder = props.searchPlaceholder ?? `搜索${title}`;
  const labelMap = React.useMemo(
    () => new Map(props.options.map((option) => [option.value, option.label])),
    [props.options],
  );
  const optionMap = React.useMemo(
    () => new Map(props.options.map((option) => [option.value, option])),
    [props.options],
  );
  const selectedLabel = (value: string) => labelMap.get(value) ?? props.unknownValueLabel ?? value;
  const items = React.useMemo(
    () => [...new Set([...props.options.map((option) => option.value), ...props.selected])],
    [props.options, props.selected],
  );
  const handleValueChange = (values: string[]) => {
    const next = props.singleSelect && values.length > 0 ? [values.at(-1)!] : values;
    props.onChange(next);
    setInputValue("");
    if (props.singleSelect) setOpen(false);
  };
  return (
    <Combobox
      multiple
      items={items}
      value={props.selected}
      onValueChange={handleValueChange}
      inputValue={inputValue}
      onInputValueChange={setInputValue}
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) {
          setInputValue("");
          setExpanded(false);
        }
      }}
      disabled={props.disabled}
      filter={(value, query) => {
        const option = { value, label: selectedLabel(value) };
        return matchesMultiSelectOption(option, query);
      }}
    >
      <ComboboxTrigger
        ref={triggerAnchorRef}
        open={open}
        className={cn(props.disabled && "pointer-events-none opacity-50", props.className)}
        aria-label={props.ariaLabel ?? title}
        aria-expanded={open}
        aria-haspopup="dialog"
        role="combobox"
        tabIndex={props.disabled ? -1 : 0}
        data-popup-open={open ? "" : undefined}
        data-disabled={props.disabled ? "" : undefined}
        onPointerDownCapture={(event) => {
          if (props.disabled) return;
          const target = event.target as HTMLElement;
          const removeButton = Boolean(target.closest('[data-slot="combobox-chip-remove"]'));
          if (!shouldCloseMultiSelectFromTriggerPress(open, event.button, removeButton)) return;
          closingFromTriggerPress.current = true;
          event.stopPropagation();
          setOpen(false);
        }}
        onClick={(event) => {
          if (props.disabled) return;
          const target = event.target as HTMLElement;
          if (target.closest('[data-slot="combobox-chip-remove"]')) return;
          if (closingFromTriggerPress.current) {
            closingFromTriggerPress.current = false;
            event.preventDefault();
            event.stopPropagation();
            return;
          }
          setOpen(true);
        }}
        onKeyDown={(event) => {
          if (props.disabled) return;
          if (!["Enter", " ", "ArrowDown"].includes(event.key)) return;
          event.preventDefault();
          setOpen((current) => (event.key === "ArrowDown" ? true : !current));
        }}
      >
        <ComboboxChips className="contents">
          <ComboboxValue>
            {(values: string[]) => {
              const shouldLimit = typeof props.maxVisibleChips === "number" && !expanded;
              const visibleValues = shouldLimit ? values.slice(0, props.maxVisibleChips) : values;
              const hiddenCount = values.length - visibleValues.length;
              return (
                <>
                  {values.length === 0 && showTitle ? (
                    <span className="text-muted-foreground min-w-0 flex-1 truncate text-left">
                      {title}
                    </span>
                  ) : null}
                  {visibleValues.map((value) => (
                    <ComboboxChip key={value}>
                      <span className="max-w-64 truncate">{selectedLabel(value)}</span>
                    </ComboboxChip>
                  ))}
                  {hiddenCount > 0 ? (
                    <button
                      type="button"
                      className="bg-muted text-muted-foreground hover:bg-muted/80 h-5 rounded-sm px-1.5 text-xs font-medium whitespace-nowrap"
                      onPointerDown={(event) => event.stopPropagation()}
                      onClick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        setExpanded(true);
                      }}
                    >
                      另有 {hiddenCount} 个
                    </button>
                  ) : null}
                </>
              );
            }}
          </ComboboxValue>
        </ComboboxChips>
      </ComboboxTrigger>
      <ComboboxContent anchor={triggerAnchorRef}>
        <ComboboxSearch
          placeholder={searchPlaceholder}
          aria-label={searchPlaceholder}
          disabled={props.disabled}
        />
        <ComboboxList>
          <ComboboxCollection>
            {(value: string) => (
              <ComboboxItem key={value} value={value}>
                {props.renderOption ? (
                  props.renderOption(optionMap.get(value) ?? { value, label: selectedLabel(value) })
                ) : (
                  <span className="truncate">{selectedLabel(value)}</span>
                )}
              </ComboboxItem>
            )}
          </ComboboxCollection>
        </ComboboxList>
        <ComboboxEmpty>{props.emptyText ?? "没有匹配的选项"}</ComboboxEmpty>
        <MultiSelectClearAction
          text={props.clearText ?? "清空筛选"}
          disabled={props.selected.length === 0 && inputValue.length === 0}
          onClear={() => {
            props.onChange([]);
            setInputValue("");
            setExpanded(false);
            setOpen(false);
          }}
        />
      </ComboboxContent>
    </Combobox>
  );
}
