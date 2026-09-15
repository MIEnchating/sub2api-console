import type { ReactElement } from "react";
import { LoaderCircle } from "lucide-react";

export function StartupLoading(props: { label: string }): ReactElement {
  return (
    <main
      role="status"
      aria-label={props.label}
      aria-busy="true"
      className="bg-background text-foreground grid min-h-svh place-items-center px-6 py-8"
    >
      <div className="flex w-full max-w-xs flex-col items-center gap-5 text-center">
        <img src="/console-mark.svg" width={56} height={56} alt="" className="size-14 shrink-0" />
        <div className="grid gap-1.5">
          <h1 className="text-lg font-semibold">Sub2API Console</h1>
          <p className="text-muted-foreground flex items-center justify-center gap-2 text-sm">
            <LoaderCircle
              className="size-3.5 shrink-0 motion-safe:animate-spin"
              aria-hidden="true"
            />
            {props.label}
          </p>
        </div>
      </div>
    </main>
  );
}
