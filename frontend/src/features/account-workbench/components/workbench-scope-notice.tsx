import type { ReactElement } from "react";

export function WorkbenchScopeNotice(props: {
  scope?: "local-export";
  children: string;
}): ReactElement {
  const local = props.scope === "local-export";
  return (
    <p role="note" className="text-xs leading-5 text-muted-foreground">
      <span className="font-medium">{local ? "本地私有范围" : "线上托管范围"}</span>
      <span className="ml-2 text-muted-foreground">{props.children}</span>
    </p>
  );
}
