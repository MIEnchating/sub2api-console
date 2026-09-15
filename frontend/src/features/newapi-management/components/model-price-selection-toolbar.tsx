import { useId, type ReactElement } from "react";
import { ListChecks, RefreshCw } from "lucide-react";

import { SelectionToolbar } from "@/components/data-table/selection-toolbar";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function ModelPriceSelectionToolbar(props: {
  selectedCount: number;
  pending: boolean;
  selectAllDisabled: boolean;
  onClear: () => void;
  onSelectAll: () => void;
  onSync: () => void;
}): ReactElement {
  const limitId = useId();
  const overLimit = props.selectedCount > 1000;

  return (
    <SelectionToolbar
      selectedCount={props.selectedCount}
      entityLabel="模型"
      pending={props.pending}
      onClear={props.onClear}
      message={
        overLimit && (
          <p
            id={limitId}
            role="status"
            className="mt-2 border-t pt-2 text-center text-xs text-destructive wrap-anywhere"
          >
            每批最多同步 1000 个模型
          </p>
        )
      }
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label="全选筛选结果"
              disabled={props.pending || props.selectAllDisabled}
              onClick={props.onSelectAll}
            />
          }
        >
          <ListChecks aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>全选筛选结果</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={`批量同步（${props.selectedCount}）`}
              aria-describedby={overLimit ? limitId : undefined}
              disabled={props.pending || overLimit}
              onClick={props.onSync}
            />
          }
        >
          <RefreshCw aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>同步已选择模型价格</TooltipContent>
      </Tooltip>
    </SelectionToolbar>
  );
}
