import type { ReactElement } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** 错误原因由统一 toast 展示，此处仅提供恢复入口。 */
export function ContentRetry(props: {
  onRetry: () => void;
  pending?: boolean;
  compact?: boolean;
}): ReactElement {
  return (
    <div
      className={cn(
        "flex min-h-24 min-w-0 items-center justify-center p-4",
        props.compact && "min-h-8 justify-start p-0",
      )}
    >
      <Button type="button" variant="outline" onClick={props.onRetry} disabled={props.pending}>
        <RefreshCw aria-hidden="true" />
        重新读取
      </Button>
    </div>
  );
}
