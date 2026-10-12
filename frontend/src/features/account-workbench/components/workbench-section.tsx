import type { ReactElement, ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

export function WorkbenchToolbar(props: {
  description: string;
  meta?: ReactNode;
  actions: ReactNode;
  children?: ReactNode;
}): ReactElement {
  return (
    <div
      role="group"
      aria-label="工作台操作"
      className="flex min-w-0 flex-wrap items-center gap-3 rounded-lg border bg-card p-3 shadow-xs sm:p-4"
    >
      <p className="sr-only">{props.description}</p>
      {props.children && <div className="min-w-0 flex-1 basis-80">{props.children}</div>}
      {props.meta}
      <div
        role="group"
        aria-label="页面操作"
        className="ml-auto flex min-w-0 flex-wrap items-center gap-2"
      >
        {props.actions}
      </div>
    </div>
  );
}

export function WorkbenchEmptyState(props: {
  icon: LucideIcon;
  title: string;
  description?: string;
}): ReactElement {
  return (
    <div className="grid min-w-0 justify-items-center gap-3 rounded-lg border bg-card px-4 py-12 text-center">
      <span className="mb-1 flex size-12 items-center justify-center rounded-lg border bg-muted/40 text-muted-foreground">
        <props.icon className="size-5" aria-hidden="true" />
      </span>
      <p className="text-sm font-medium wrap-anywhere">{props.title}</p>
      {props.description && (
        <p className="max-w-md text-xs leading-5 text-muted-foreground">{props.description}</p>
      )}
    </div>
  );
}
