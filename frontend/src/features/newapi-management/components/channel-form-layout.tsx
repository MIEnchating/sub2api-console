import type { ReactElement, ReactNode } from "react";
import { CardContent } from "@/components/ui/card";

export function ChannelFormColumns(props: {
  kind: "credentials" | "configuration";
  children: ReactNode;
}): ReactElement {
  return (
    <CardContent
      data-channel-credentials-layout={props.kind === "credentials" ? "" : undefined}
      data-channel-configuration-layout={props.kind === "configuration" ? "" : undefined}
      className="grid grid-cols-1 gap-4 @3xl/channel:grid-cols-2"
    >
      {props.children}
    </CardContent>
  );
}

export function ChannelFormFooter(props: { note: string; children: ReactNode }): ReactElement {
  return (
    <div
      data-slot="channel-form-footer"
      className="flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-3 border-t border-border/70 px-4 py-3"
    >
      <p className="min-w-0 flex-1 basis-48 text-xs leading-5 text-muted-foreground">
        {props.note}
      </p>
      <div className="ml-auto flex shrink-0 items-center gap-2">{props.children}</div>
    </div>
  );
}
