import { useId, type ReactElement } from "react";

import { FieldLabel } from "@/components/field-help-tooltip";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

export function PolicySwitchRow(props: {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  id?: string;
  ariaLabel?: string;
  onCheckedChange: (value: boolean) => void;
}): ReactElement {
  const generatedId = useId();
  const id = props.id ?? generatedId;

  return (
    <div className="border-border/60 bg-muted/20 flex min-h-14 min-w-0 items-center justify-between gap-3 rounded-lg border px-3 py-3">
      <FieldLabel
        label={props.label}
        description={props.description}
        htmlFor={id}
        className={cn(
          "text-sm [overflow-wrap:anywhere]",
          props.disabled ? "cursor-not-allowed" : "cursor-pointer",
        )}
      />
      {props.ariaLabel ? (
        <span id={`${id}-toggle-name`} className="sr-only">
          {props.ariaLabel}
        </span>
      ) : null}
      <Switch
        id={id}
        checked={props.checked}
        disabled={props.disabled}
        aria-label={props.ariaLabel ?? props.label}
        aria-labelledby={props.ariaLabel ? `${id}-toggle-name` : undefined}
        onCheckedChange={props.onCheckedChange}
      />
    </div>
  );
}
