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
      className="min-w-0 rounded-lg border bg-card shadow-xs"
    >
      <div className="flex min-w-0 flex-col gap-3 p-3 sm:flex-row sm:flex-wrap sm:items-center sm:p-4">
        <div className="mr-auto flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <p className="text-sm leading-6 text-muted-foreground wrap-anywhere">
            {props.description}
          </p>
          {props.meta}
        </div>
        <div
          role="group"
          aria-label="页面操作"
          className="flex min-w-0 flex-wrap items-center gap-2"
        >
          {props.actions}
        </div>
      </div>
      {props.children && <div className="min-w-0 border-t p-3 sm:px-4">{props.children}</div>}
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
