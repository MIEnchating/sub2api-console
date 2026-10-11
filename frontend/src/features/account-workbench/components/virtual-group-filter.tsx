import { useLayoutEffect, useMemo, useRef, useState, type ReactElement } from "react";
import { Combobox as ComboboxPrimitive } from "@base-ui/react/combobox";
import { useVirtualizer } from "@tanstack/react-virtual";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ComboboxContent, ComboboxItem, ComboboxSearch } from "@/components/ui/combobox";
import type { WorkbenchAccount } from "../types";

export function VirtualGroupFilter(props: {
  groups: ReadonlyMap<string, WorkbenchAccount["groups"][number]>;
  value: string;
  onChange: (value: string) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const anchor = useRef<HTMLButtonElement>(null);
  const [scroller, setScroller] = useState<HTMLDivElement | null>(null);
  const options = useMemo(
    () => [
      { value: "all", label: "全部分组", words: "all 全部分组" },
      ...Array.from(props.groups.values(), (group) => ({
        value: group.id,
        label: group.name || `分组 #${group.id}`,
        words: `${group.id} ${group.name || `分组 #${group.id}`}`.toLocaleLowerCase(),
      })),
    ],
    [props.groups],
  );
  const filtered = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    return query ? options.filter((option) => option.words.includes(query)) : options;
  }, [options, search]);
  const values = useMemo(() => options.map((option) => option.value), [options]);
  const filteredValues = useMemo(() => filtered.map((option) => option.value), [filtered]);
  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => scroller,
    estimateSize: () => 32,
    getItemKey: (index) => filtered[index].value,
    overscan: 5,
    enabled: open,
    initialRect: { width: 192, height: 288 },
  });
  useLayoutEffect(() => {
    if (!open || !scroller || filteredValues.length === 0) return;
    const selected = search ? 0 : filteredValues.indexOf(props.value);
    virtualizer.scrollToIndex(Math.max(0, selected), { align: "auto" });
  }, [open, scroller, filteredValues, props.value, search, virtualizer]);
  const label =
    props.value === "all"
      ? "全部分组"
      : props.groups.get(props.value)?.name || `分组 #${props.value}`;
  return (
    <ComboboxPrimitive.Root
      virtualized
      items={values}
      filteredItems={filteredValues}
      value={props.value}
      inputValue={search}
      onInputValueChange={(value, details) => {
        if (details.reason === "input-change" || details.reason === "input-clear") setSearch(value);
      }}
      open={open}
      onOpenChange={(value) => {
        setOpen(value);
        if (!value) setSearch("");
      }}
      onValueChange={(value, details) => {
        if (details.reason === "none" || value === null) return;
        if (value === "all" || props.groups.has(value)) props.onChange(value);
      }}
      onItemHighlighted={(_, details) => {
        if (details.index >= 0 && details.reason === "keyboard")
          virtualizer.scrollToIndex(details.index, { align: "auto" });
      }}
    >
      <ComboboxPrimitive.Trigger
        ref={anchor}
        role="combobox"
        aria-label="筛选账号分组"
        render={<Button variant="outline" className="w-full min-w-0 justify-between font-normal" />}
      >
        <span className="min-w-0 truncate">{label}</span>
        <ChevronDown aria-hidden="true" className="shrink-0" />
      </ComboboxPrimitive.Trigger>
      <ComboboxContent anchor={anchor}>
        <ComboboxSearch aria-label="搜索选项" placeholder="搜索选项" />
        <ComboboxPrimitive.List
          ref={setScroller}
          aria-label="账号分组"
          className="relative max-h-72 min-h-0 overflow-y-auto overscroll-contain"
        >
          {filtered.length === 0 ? (
            <div className="py-6 text-center text-sm text-muted-foreground">没有匹配的选项</div>
          ) : (
            <div
              role="presentation"
              className="relative w-full"
              style={{ height: virtualizer.getTotalSize() }}
            >
              {virtualizer.getVirtualItems().map((item) => (
                <ComboboxItem
                  key={item.key}
                  ref={virtualizer.measureElement}
                  data-index={item.index}
                  index={item.index}
                  value={filtered[item.index].value}
                  aria-posinset={item.index + 1}
                  aria-setsize={filtered.length}
                  className="absolute top-0 left-0 min-h-8 whitespace-normal wrap-anywhere"
                  style={{ transform: `translateY(${item.start}px)` }}
                >
                  {filtered[item.index].label}
                </ComboboxItem>
              ))}
            </div>
          )}
        </ComboboxPrimitive.List>
      </ComboboxContent>
    </ComboboxPrimitive.Root>
  );
}
