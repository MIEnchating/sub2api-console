import type { ReactElement } from "react";
import { cn } from "@/lib/utils";

export function FieldError(props: {
  message?: string | null;
  id?: string;
  className?: string;
  reserveSpace?: boolean;
}): ReactElement | null {
  if (!props.message && !props.reserveSpace) return null;
  return (
    <span
      id={props.id}
      data-slot="field-error"
      className={cn(
        "text-destructive block min-w-0 shrink-0 text-xs leading-4 font-normal wrap-anywhere",
        props.reserveSpace && "min-h-8",
        props.className,
      )}
      role={props.message ? "alert" : undefined}
      aria-hidden={props.message ? undefined : true}
    >
      {props.message}
    </span>
  );
}
