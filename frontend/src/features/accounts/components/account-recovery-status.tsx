import type { AccountRecovery } from "@/api";
import type { ReactElement } from "react";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";

export function AccountRecoveryStatus(props: {
  recovery: AccountRecovery;
  expanded?: boolean;
}): ReactElement {
  const unmet = props.recovery.conditions.filter((condition) => !condition.met);
  if (!props.expanded) {
    const summary = props.recovery.ready
      ? "恢复条件已满足，等待调度执行"
      : `恢复待满足：${unmet.map((condition) => condition.detail).join("；")}`;
    return (
      <TableOverflowTooltip content={summary} className="text-xs text-warning">
        {summary}
      </TableOverflowTooltip>
    );
  }
  return (
    <section aria-label="自动恢复条件" className="grid min-w-0 gap-2 rounded-lg border p-3">
      <p className="font-medium">自动恢复条件</p>
      <ul className="grid gap-2 text-sm">
        {props.recovery.conditions.map((condition) => (
          <li key={condition.code} className="flex min-w-0 items-start gap-2">
            <span className={condition.met ? "shrink-0 text-success" : "shrink-0 text-warning"}>
              {condition.met ? "已满足" : "未满足"}
            </span>
            <span className="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
              {condition.detail}
            </span>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">
        评估时间：{props.recovery.evaluated_at}。进度以该次评估为准，下一轮探活和调度后更新。
      </p>
      {props.recovery.ready ? (
        <p className="text-sm text-warning">
          恢复条件已满足，等待调度执行；请以调度开关和执行结果为准。
        </p>
      ) : null}
    </section>
  );
}
