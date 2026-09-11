import type { ReactElement } from "react";
import { LoaderCircle } from "lucide-react";

import { cn } from "@/lib/utils";

/** 弹窗及局部内容读取；页面首次加载使用 PageLoadingSkeleton。 */
export function ContentLoading(props: {
  label: string;
  compact?: boolean;
  className?: string;
}): ReactElement {
  return (
    <div
      role="status"
      aria-label={props.label}
      aria-busy="true"
      className={cn(
        "text-muted-foreground flex min-h-40 min-w-0 flex-col items-center justify-center gap-3 px-4 py-6 text-center text-sm",
        props.compact && "min-h-8 flex-row justify-start gap-2 px-0 py-1 text-left text-xs",
        props.className,
      )}
    >
      <LoaderCircle
        className={cn("size-5 shrink-0 motion-safe:animate-spin", props.compact && "size-4")}
        aria-hidden="true"
      />
      <span className="min-w-0 max-w-full break-words">{props.label}</span>
    </div>
  );
}
