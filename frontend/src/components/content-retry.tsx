import type { ReactElement } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";

/** 错误原因由统一 toast 展示，此处仅提供恢复入口。 */
export function ContentRetry(props: { onRetry: () => void; pending?: boolean }): ReactElement {
  return (
    <div className="flex min-h-40 items-center justify-center p-4">
      <Button type="button" variant="outline" onClick={props.onRetry} disabled={props.pending}>
        <RefreshCw aria-hidden="true" />
        重新读取
      </Button>
    </div>
  );
}
