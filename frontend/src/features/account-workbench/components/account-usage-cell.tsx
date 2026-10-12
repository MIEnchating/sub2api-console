import type { ReactElement } from "react";
import type { AccountUsage, AccountUsageWindow } from "../types";

function UsageWindow(props: { label: string; window?: AccountUsageWindow }): ReactElement {
  const value = props.window;
  const expired = value?.reset_at && Date.parse(value.reset_at) <= Date.now();
  let content: ReactElement = <span className="text-muted-foreground">暂无数据</span>;
  if (value && expired) {
    content = <span className="text-muted-foreground">待刷新（上次 {value.used_percent}%）</span>;
  } else if (value) {
    content = (
      <span
        role="meter"
        aria-label={props.label + " 已用"}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={value.used_percent}
        className="tabular-nums"
      >
        {value.used_percent}%
      </span>
    );
  }
  return (
    <div>
      <div className="flex items-center gap-2">
        <span className="font-medium">{props.label}</span>
        {content}
      </div>
      {value?.reset_at && !expired && (
        <div className="text-xs text-muted-foreground">
          重置：{new Date(value.reset_at).toLocaleString("zh-CN")}
        </div>
      )}
    </div>
  );
}

export function AccountUsageCell(props: { usage?: AccountUsage }): ReactElement {
  return (
    <div className="grid gap-1 text-xs">
      <UsageWindow label="5h" window={props.usage?.five_hour} />
      <UsageWindow label="7d" window={props.usage?.seven_day} />
      {props.usage?.updated_at && (
        <div className="text-muted-foreground">
          更新于 {new Date(props.usage.updated_at).toLocaleString("zh-CN")}
        </div>
      )}
    </div>
  );
}
