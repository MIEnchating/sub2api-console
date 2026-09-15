import { SelectionToolbar } from "@/components/data-table/selection-toolbar";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { groupBatchActionOrder, groupBatchActions, type GroupBatchAction } from "../constants";

export function GroupSelectionToolbar(props: {
  selectedCount: number;
  pending: boolean;
  disabled: boolean;
  onClear: () => void;
  onAction: (action: GroupBatchAction) => void;
}) {
  return (
    <SelectionToolbar
      selectedCount={props.selectedCount}
      entityLabel="分组"
      pending={props.pending}
      onClear={props.onClear}
    >
      {groupBatchActionOrder.map((action) => {
        const meta = groupBatchActions[action];
        const Icon = meta.icon;
        return (
          <Tooltip key={action}>
            <TooltipTrigger
              render={
                <Button
                  variant={action === "exclude" ? "destructive" : "outline"}
                  size="icon"
                  aria-label={meta.label}
                  disabled={props.pending || props.disabled}
                  onClick={() => props.onAction(action)}
                />
              }
            >
              <Icon aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>{meta.label}</TooltipContent>
          </Tooltip>
        );
      })}
    </SelectionToolbar>
  );
}
