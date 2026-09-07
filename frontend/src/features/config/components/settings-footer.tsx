import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

export function SettingsFooter(props: { children: ReactNode; className?: string }) {
  return (
    <div
      data-slot="settings-footer"
      className={cn(
        "border-border/70 flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-3 py-3",
        props.className,
      )}
    >
      {props.children}
    </div>
  );
}
