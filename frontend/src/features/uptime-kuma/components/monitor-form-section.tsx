import type { ReactNode } from "react";

export function MonitorFormSection(props: { title: string; children: ReactNode }) {
  return (
    <section aria-label={props.title} className="grid min-w-0 gap-3">
      <h3 className="border-b pb-2 text-sm font-medium">{props.title}</h3>
      {props.children}
    </section>
  );
}
