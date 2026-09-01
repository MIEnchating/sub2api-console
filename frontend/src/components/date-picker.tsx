/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { CalendarDate, getLocalTimeZone, today } from "@internationalized/date";
import { CalendarDays, X } from "lucide-react";
import { useCallback, useRef, useState } from "react";
import {
  Button,
  DatePicker as ReactAriaDatePicker,
  Group,
  I18nProvider,
} from "react-aria-components";

import { AriaCalendarPopover, SegmentedDateInput } from "@/components/aria-date-primitives";
import {
  getCalendarPopoverPlacement,
  type CalendarPopoverPlacement,
} from "@/components/date-time-picker-utils";
import { cn } from "@/lib/utils";

type DatePickerProps = {
  selected: Date | undefined;
  onSelect: (date: Date | undefined) => void;
  label: string;
  className?: string;
  disabled?: boolean;
  fromDate?: Date;
  toDate?: Date;
  clearable?: boolean;
};

export const calendarPopoverBehavior = { controlled: true, modal: true } as const;

function toCalendarDate(date: Date | undefined) {
  return date ? new CalendarDate(date.getFullYear(), date.getMonth() + 1, date.getDate()) : null;
}

export function DatePicker(props: DatePickerProps) {
  const timeZone = getLocalTimeZone();
  const value = toCalendarDate(props.selected);
  const [portalContainer, setPortalContainer] = useState<Element>();
  const [calendarPlacement, setCalendarPlacement] =
    useState<CalendarPopoverPlacement>("bottom start");
  const [open, setOpen] = useState(false);
  const groupElementRef = useRef<HTMLDivElement | null>(null);
  const setGroupRef = useCallback((element: HTMLDivElement | null) => {
    groupElementRef.current = element;
    setPortalContainer(
      element?.closest('[data-slot="dialog-content"], [data-slot="sheet-content"]') ?? undefined,
    );
  }, []);
  const clearable = props.clearable ?? true;
  return (
    <I18nProvider locale="zh-CN">
      <ReactAriaDatePicker
        aria-label={props.label}
        value={value}
        minValue={toCalendarDate(props.fromDate) ?? undefined}
        maxValue={toCalendarDate(props.toDate) ?? undefined}
        placeholderValue={value ?? today(timeZone)}
        isDisabled={props.disabled ?? false}
        isRequired={!clearable}
        shouldCloseOnSelect
        isOpen={open}
        onOpenChange={setOpen}
        onChange={(nextValue) => {
          if (!nextValue && !clearable) return;
          props.onSelect(nextValue ? nextValue.toDate(timeZone) : undefined);
        }}
        data-trigger="DatePicker"
        className={cn("w-full min-w-0", props.className)}
      >
        <Group
          ref={setGroupRef}
          className="border-input focus-within:border-ring focus-within:ring-ring/50 dark:bg-input/30 flex h-8 w-full min-w-0 items-center rounded-lg border bg-transparent transition-colors focus-within:ring-3 data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50"
        >
          <SegmentedDateInput />
          {value && clearable ? (
            <button
              type="button"
              aria-label="清除日期"
              className="text-muted-foreground hover:text-foreground focus-visible:ring-ring flex size-7 shrink-0 items-center justify-center rounded-md outline-none focus-visible:ring-2"
              onClick={() => props.onSelect(undefined)}
            >
              <X className="size-3.5" />
            </button>
          ) : null}
          <Button
            aria-label={props.label}
            onPress={() =>
              setCalendarPlacement(getCalendarPopoverPlacement(groupElementRef.current))
            }
            className="text-muted-foreground hover:text-foreground hover:bg-accent focus-visible:ring-ring mr-0.5 flex size-7 shrink-0 items-center justify-center rounded-md outline-none focus-visible:ring-2"
          >
            <CalendarDays className="size-4" />
          </Button>
        </Group>
        <AriaCalendarPopover portalContainer={portalContainer} placement={calendarPlacement} />
      </ReactAriaDatePicker>
    </I18nProvider>
  );
}
