import { lazy, Suspense, type ReactElement } from "react";
import { ContentLoading } from "@/components/content-loading";
import { cn } from "@/lib/utils";
import type { JsonEditorProps } from "./json-editor/types";

const JsonEditorContent = lazy(() => import("./json-editor/editor"));

export function JsonEditor(props: JsonEditorProps): ReactElement {
  return (
    <div
      data-slot="json-editor"
      className={cn(
        "flex h-56 min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border border-input bg-background text-foreground font-normal [--json-property:var(--primary)] dark:[--json-property:var(--info)] focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/30 has-[.cm-json-search]:min-h-60",
        props.disabled && "bg-muted/30 opacity-70",
        props["aria-invalid"] && "border-destructive",
        props.className,
      )}
    >
      <Suspense
        fallback={<ContentLoading label="正在加载 JSON 编辑器" compact className="h-full" />}
      >
        <JsonEditorContent {...props} />
      </Suspense>
    </div>
  );
}

export type { JsonEditorProps } from "./json-editor/types";
