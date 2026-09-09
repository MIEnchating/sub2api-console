import type { ReactElement } from "react";

import { PageActions } from "@/components/page-actions";
import { PageActionMenu } from "@/components/page-action-menu";
import { RefreshButton } from "@/components/refresh-button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { groupBatchActionOrder, groupBatchActions, type GroupBatchAction } from "../constants";

export function GroupsPageActions(props: {
  refreshing: boolean;
  disabled: boolean;
  selectedCount: number;
  targetCount: number;
  onRefresh: () => void;
  onAction: (action: GroupBatchAction) => void;
}): ReactElement {
  return (
    <PageActions>
      <RefreshButton pending={props.refreshing} ariaLabel="刷新分组" onClick={props.onRefresh} />
      <PageActionMenu label="分组维护">
        <p className="px-2 py-1.5 text-xs text-muted-foreground">
          {props.selectedCount > 0
            ? `已选择 ${props.selectedCount} 个分组`
            : `当前筛选 ${props.targetCount} 个分组`}
        </p>
        {groupBatchActionOrder.map((action) => {
          const meta = groupBatchActions[action];
          const Icon = meta.icon;
          return (
            <DropdownMenuItem
              key={action}
              disabled={props.disabled || props.targetCount === 0}
              onClick={() => props.onAction(action)}
              className={
                action === "exclude" ? "text-destructive focus:text-destructive" : undefined
              }
            >
              <Icon aria-hidden="true" />
              {meta.label}
            </DropdownMenuItem>
          );
        })}
      </PageActionMenu>
    </PageActions>
  );
}
