import type { ReactElement, ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

export function WorkbenchToolbar(props: { meta?: ReactNode; actions: ReactNode }): ReactElement {
  return (
    <div className="flex min-w-0 shrink-0 flex-wrap items-center justify-end gap-2">
      {props.meta && <div className="mr-auto">{props.meta}</div>}
      {props.actions}
    </div>
  );
}

export function WorkbenchEmptyState(props: {
  icon: LucideIcon;
  title: string;
  description?: string;
}): ReactElement {
  return (
    <div className="grid min-w-0 justify-items-center gap-3 rounded-xl border border-dashed bg-card px-4 py-10 text-center">
      <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <props.icon className="size-5" aria-hidden="true" />
      </span>
      <p className="text-sm text-muted-foreground">{props.title}</p>
      {props.description && (
        <p className="max-w-md text-xs leading-5 text-muted-foreground">{props.description}</p>
      )}
    </div>
  );
}
