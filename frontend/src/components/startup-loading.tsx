import type { ReactElement } from "react";
import { LoaderCircle } from "lucide-react";

export function StartupLoading(props: { label: string }): ReactElement {
  return (
    <main
      role="status"
      aria-label={props.label}
      aria-busy="true"
      className="bg-background text-foreground grid min-h-svh place-items-center px-6"
    >
      <div className="flex w-full max-w-xs flex-col items-center gap-5 text-center">
        <div className="relative grid size-14 place-items-center rounded-2xl border border-primary/20 bg-primary/10 text-primary shadow-sm">
          <LoaderCircle className="size-7 motion-safe:animate-spin" aria-hidden="true" />
          <span className="sr-only">{props.label}</span>
        </div>
        <div className="grid gap-1.5">
          <h1 className="font-display text-lg font-semibold tracking-tight">Sub2API Console</h1>
          <p className="text-muted-foreground text-sm">{props.label}</p>
        </div>
        <div className="flex items-center gap-1" aria-hidden="true">
          <span className="size-1.5 animate-pulse rounded-full bg-primary" />
          <span className="size-1.5 animate-pulse rounded-full bg-primary [animation-delay:150ms]" />
          <span className="size-1.5 animate-pulse rounded-full bg-primary [animation-delay:300ms]" />
        </div>
      </div>
    </main>
  );
}
