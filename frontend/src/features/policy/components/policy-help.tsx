import type { ReactElement, ReactNode } from "react";

export function PolicyHelp(props: { label: string; children: ReactNode }): ReactElement {
  return (
    <details className="group text-muted-foreground text-xs leading-6">
      <summary className="focus-visible:ring-ring w-fit cursor-pointer rounded-sm font-medium outline-none focus-visible:ring-2">
        {props.label}
      </summary>
      <div className="bg-muted/40 mt-2 space-y-2 rounded-lg px-3 py-2">{props.children}</div>
    </details>
  );
}
