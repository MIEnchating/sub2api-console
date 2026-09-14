import type { ReactElement } from "react";
import { cn } from "@/lib/utils";

export function FieldError(props: {
  message?: string | null;
  id?: string;
  className?: string;
  floating?: boolean;
}): ReactElement {
  if (props.floating) {
    return (
      <span data-slot="field-error" className="relative block h-0 min-w-0">
        {props.message ? (
          <span
            id={props.id}
            role="alert"
            tabIndex={0}
            className={cn(
              "text-destructive absolute top-0 left-0 z-20 mt-1 block max-h-24 w-full overflow-y-auto overscroll-contain rounded-md border bg-popover px-2 py-1 text-xs leading-4 font-normal shadow-md wrap-anywhere focus-visible:outline-2 focus-visible:outline-offset-2",
              props.className,
            )}
          >
            {props.message}
          </span>
        ) : null}
      </span>
    );
  }
  return (
    <span
      id={props.id}
      data-slot="field-error"
      className={cn(
        "text-destructive block h-8 min-h-8 min-w-0 shrink-0 overflow-y-auto text-xs leading-4 font-normal wrap-anywhere focus-visible:outline-2 focus-visible:outline-offset-2",
        props.className,
      )}
      role={props.message ? "alert" : undefined}
      tabIndex={props.message ? 0 : undefined}
      aria-hidden={props.message ? undefined : true}
    >
      {props.message}
    </span>
  );
}
