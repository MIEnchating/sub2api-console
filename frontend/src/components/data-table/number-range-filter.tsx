import { useId } from "react";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export type NumberRangeFilterProps = {
  label: string;
  minimumValue: string;
  maximumValue: string;
  onMinimumValueChange: (value: string) => void;
  onMaximumValueChange: (value: string) => void;
  minimumPlaceholder?: string;
  maximumPlaceholder?: string;
  minimumAriaLabel?: string;
  maximumAriaLabel?: string;
  min?: number | string;
  max?: number | string;
  step?: number | string;
  disabled?: boolean;
  error?: string;
  className?: string;
  inputClassName?: string;
};

export function NumberRangeFilter(props: NumberRangeFilterProps) {
  const errorID = useId();
  return (
    <div
      role="group"
      aria-label={`${props.label}范围`}
      data-slot="number-range-filter"
      className={cn("flex shrink-0 flex-col gap-1.5", props.className)}
    >
      <div className="flex items-center gap-1.5">
        <span className="text-muted-foreground shrink-0 text-sm">{props.label}</span>
        <Input
          type="number"
          inputMode="decimal"
          min={props.min}
          max={props.max}
          step={props.step}
          value={props.minimumValue}
          onChange={(event) => props.onMinimumValueChange(event.target.value)}
          placeholder={props.minimumPlaceholder ?? "最低"}
          aria-label={props.minimumAriaLabel ?? `最低${props.label}`}
          aria-invalid={Boolean(props.error)}
          aria-describedby={props.error ? errorID : undefined}
          disabled={props.disabled}
          className={cn("w-28", props.inputClassName)}
        />
        <span className="text-muted-foreground text-sm" aria-hidden="true">
          至
        </span>
        <Input
          type="number"
          inputMode="decimal"
          min={props.min}
          max={props.max}
          step={props.step}
          value={props.maximumValue}
          onChange={(event) => props.onMaximumValueChange(event.target.value)}
          placeholder={props.maximumPlaceholder ?? "最高"}
          aria-label={props.maximumAriaLabel ?? `最高${props.label}`}
          aria-invalid={Boolean(props.error)}
          aria-describedby={props.error ? errorID : undefined}
          disabled={props.disabled}
          className={cn("w-28", props.inputClassName)}
        />
      </div>
      {props.error ? (
        <span id={errorID} role="alert" className="text-destructive text-xs">
          {props.error}
        </span>
      ) : null}
    </div>
  );
}
