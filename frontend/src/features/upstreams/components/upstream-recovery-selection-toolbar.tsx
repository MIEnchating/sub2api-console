import { ShieldCheck } from "lucide-react";

import { SelectionToolbar } from "@/components/data-table/selection-toolbar";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function UpstreamRecoverySelectionToolbar(props: {
  selectedCount: number;
  pending: boolean;
  onClear: () => void;
  onRecover: () => void;
}) {
  return (
    <SelectionToolbar
      selectedCount={props.selectedCount}
      entityLabel="上游"
      pending={props.pending}
      onClear={props.onClear}
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              size="icon"
              aria-label={`恢复已选择的 ${props.selectedCount} 个上游鉴权`}
              disabled={props.pending}
              onClick={props.onRecover}
            />
          }
        >
          <ShieldCheck aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>恢复已选择上游鉴权</TooltipContent>
      </Tooltip>
    </SelectionToolbar>
  );
}
