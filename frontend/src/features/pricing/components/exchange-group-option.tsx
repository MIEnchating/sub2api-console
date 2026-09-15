import type { ReactElement } from "react";

import type { PricingGroup } from "@/api";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";
import { GroupMinimumField } from "./group-minimum-field";

export function ExchangeGroupOption(props: {
  group: PricingGroup;
  setIndex: number;
  selected: boolean;
  assignedSetName?: string;
  wrongPlatform: boolean;
  minimum?: string;
  onToggle: (setIndex: number, groupID: string, checked: boolean) => void;
  onMinimumChange: (groupID: string, value: string) => void;
}): ReactElement {
  const disabled =
    !props.selected &&
    Boolean(props.assignedSetName || !props.group.available || props.wrongPlatform);
  let unavailableReason: string | undefined;
  if (!props.group.available) unavailableReason = props.group.reason || "分组当前不可用";
  if (props.assignedSetName) unavailableReason = `已加入 ${props.assignedSetName}`;
  if (props.wrongPlatform) unavailableReason = "与当前互换组的平台不同";
  const descriptionID = `exchange-set-${props.setIndex + 1}-group-${props.group.id}-description`;
  let footer: ReactElement | null = null;
  if (props.selected) {
    footer = (
      <GroupMinimumField
        groupID={props.group.id}
        groupName={props.group.name}
        value={props.minimum}
        onChange={props.onMinimumChange}
      />
    );
  } else if (!disabled) {
    footer = (
      <div className="border-t bg-muted/10 px-3.5 py-3 text-xs text-muted-foreground">
        选中后可设置最低迁入倍率
      </div>
    );
  }

  return (
    <div
      data-slot="exchange-group-card"
      data-selected={props.selected ? "true" : "false"}
      className={cn(
        "flex h-full min-w-0 flex-col overflow-hidden rounded-lg border text-sm transition-[border-color,background-color,box-shadow] focus-within:ring-2 focus-within:ring-ring",
        props.selected
          ? "border-primary/70 bg-primary/10 shadow-sm shadow-primary/10"
          : "border-border/80 bg-muted/20",
        disabled ? "bg-muted/30 text-muted-foreground" : "hover:border-primary/50 hover:shadow-sm",
      )}
    >
      <label
        data-slot="exchange-group-option"
        data-selected={props.selected ? "true" : "false"}
        className={cn(
          "flex min-h-[4.75rem] min-w-0 items-start gap-2.5 px-3.5 py-3",
          disabled ? "cursor-not-allowed" : "cursor-pointer",
        )}
      >
        <Checkbox
          className="mt-0.5"
          checked={props.selected}
          disabled={disabled}
          onCheckedChange={(checked) => props.onToggle(props.setIndex, props.group.id, checked)}
          aria-label={`互换组 ${props.setIndex + 1} 分组 ${props.group.name}`}
          aria-describedby={unavailableReason ? descriptionID : undefined}
        />
        <span className="grid min-w-0 flex-1 gap-1">
          <span className="min-w-0 font-medium leading-5 [overflow-wrap:anywhere]">
            {props.group.name}
          </span>
          <span
            data-slot="exchange-group-metadata"
            className="text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs leading-4 tabular-nums [overflow-wrap:anywhere]"
          >
            <span>#{props.group.id}</span>
            {props.group.rate_multiplier ? <span>售价 {props.group.rate_multiplier}</span> : null}
          </span>
          {unavailableReason ? (
            <span
              id={descriptionID}
              className="text-muted-foreground text-xs leading-5 [overflow-wrap:anywhere]"
            >
              {unavailableReason}
            </span>
          ) : null}
        </span>
      </label>
      {footer}
    </div>
  );
}
