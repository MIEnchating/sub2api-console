/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useCallback, useState } from "react";
import {
  Button,
  Calendar,
  CalendarCell,
  CalendarGrid,
  CalendarGridBody,
  CalendarGridHeader,
  CalendarHeaderCell,
  CalendarHeading,
  CalendarMonthPicker,
  CalendarYearPicker,
  DateInput,
  DateSegment,
  Dialog,
  Popover,
} from "react-aria-components";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

type CalendarPickerItem = { id: number; formatted: string };

export function SegmentedDateInput(props: { className?: string }) {
  return (
    <DateInput
      className={cn("flex min-w-0 flex-1 items-center px-3 py-1.5 tabular-nums", props.className)}
    >
      {(segment) => (
        <DateSegment
          segment={segment}
          className="data-[focused]:bg-accent data-[focused]:text-accent-foreground data-[placeholder]:text-muted-foreground rounded-sm px-0.5 outline-none data-[type=literal]:px-0"
        />
      )}
    </DateInput>
  );
}

export function AriaCalendarPopover(props: {
  portalContainer?: Element;
  placement?: "top start" | "bottom start";
}) {
  const [openPicker, setOpenPicker] = useState<"year" | "month" | null>(null);
  return (
    <Popover
      UNSTABLE_portalContainer={props.portalContainer}
      placement={props.placement ?? "bottom start"}
      shouldFlip={false}
      offset={4}
      className="bg-popover text-popover-foreground ring-foreground/10 data-[entering]:animate-in data-[entering]:fade-in-0 data-[entering]:zoom-in-95 data-[exiting]:animate-out data-[exiting]:fade-out-0 data-[exiting]:zoom-out-95 z-50 max-h-[calc(100dvh-1rem)] w-[20rem] max-w-[calc(100vw-1rem)] overflow-y-auto overscroll-contain rounded-lg shadow-md ring-1 outline-none"
    >
      <Dialog className="outline-none">
        <Calendar className="w-full p-3">
          <CalendarHeading className="sr-only" />
          <div className="mb-2 flex h-9 items-center justify-between gap-1">
            <Button
              slot="previous"
              aria-label="上个月"
              className="hover:bg-accent focus-visible:ring-ring flex size-8 items-center justify-center rounded-md outline-none focus-visible:ring-2 disabled:opacity-40"
            >
              <ChevronLeft className="size-4" />
            </Button>
            <div className="flex min-w-0 items-center justify-center gap-1">
              <CalendarYearPicker visibleYears={200}>
                {(pickerProps) => (
                  <CalendarPickerSelect
                    {...pickerProps}
                    open={openPicker === "year"}
                    onOpenChange={(open) => setOpenPicker(open ? "year" : null)}
                    wide
                  />
                )}
              </CalendarYearPicker>
              <CalendarMonthPicker format="long">
                {(pickerProps) => (
                  <CalendarPickerSelect
                    {...pickerProps}
                    open={openPicker === "month"}
                    onOpenChange={(open) => setOpenPicker(open ? "month" : null)}
                  />
                )}
              </CalendarMonthPicker>
            </div>
            <Button
              slot="next"
              aria-label="下个月"
              className="hover:bg-accent focus-visible:ring-ring flex size-8 items-center justify-center rounded-md outline-none focus-visible:ring-2 disabled:opacity-40"
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
          <CalendarGrid weekdayStyle="short" className="w-full border-separate border-spacing-y-1">
            <CalendarGridHeader>
              {(day) => (
                <CalendarHeaderCell className="text-muted-foreground h-8 text-center text-xs font-normal">
                  {day}
                </CalendarHeaderCell>
              )}
            </CalendarGridHeader>
            <CalendarGridBody>
              {(date) => (
                <CalendarCell
                  date={date}
                  className={({ isHovered, isSelected, isToday }) =>
                    cn(
                      "focus-visible:ring-ring flex size-9 items-center justify-center rounded-md text-sm outline-none focus-visible:ring-2 data-[disabled]:pointer-events-none data-[disabled]:opacity-40 data-[outside-visible-range]:invisible data-[today]:font-semibold",
                      isHovered && !isSelected && "bg-accent",
                      isToday && !isSelected && "bg-muted",
                      isSelected && "bg-primary text-primary-foreground",
                    )
                  }
                />
              )}
            </CalendarGridBody>
          </CalendarGrid>
        </Calendar>
      </Dialog>
    </Popover>
  );
}

function CalendarPickerSelect(props: {
  "aria-label": string;
  value: string | number;
  onChange: (key: string | number | null) => void;
  items: CalendarPickerItem[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wide?: boolean;
}) {
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(null);
  const setTriggerRef = useCallback((element: HTMLButtonElement | null) => {
    setPortalContainer(element?.closest<HTMLElement>('[data-trigger="DatePicker"]') ?? null);
  }, []);
  return (
    <Select
      open={props.open}
      onOpenChange={props.onOpenChange}
      value={String(props.value)}
      items={props.items.map((item) => ({ value: String(item.id), label: item.formatted }))}
      itemToStringLabel={(value) =>
        props.items.find((item) => String(item.id) === String(value))?.formatted ?? String(value)
      }
      onValueChange={(nextValue) => props.onChange(Number(nextValue))}
    >
      <SelectTrigger
        ref={setTriggerRef}
        aria-label={props["aria-label"]}
        size="sm"
        appearance="classic"
        className={cn(
          "hover:bg-accent h-8 border-0 bg-transparent px-2 font-medium tabular-nums shadow-none focus-visible:ring-2",
          props.wide ? "min-w-[5rem]" : "min-w-[4.5rem]",
        )}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        portalContainer={portalContainer}
        align="start"
        alignItemWithTrigger={false}
        appearance="classic"
        className={cn("max-h-64", props.wide ? "min-w-24" : "min-w-28")}
      >
        {props.items.map((item) => (
          <SelectItem key={item.id} value={String(item.id)} appearance="classic">
            {item.formatted}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
