import { RefreshCw } from "lucide-react";

import { Progress } from "@/components/ui/progress";

type Props = {
  message: string;
};

export const taskStartupStateLayout = {
  root: "grid min-h-12 gap-3 py-2 text-sm",
  heading: "flex min-w-0 items-center gap-2",
} as const;

export function TaskStartupState(props: Props) {
  return (
    <div
      className={taskStartupStateLayout.root}
      role="status"
      aria-live="polite"
      aria-label={props.message}
    >
      <div className={taskStartupStateLayout.heading}>
        <RefreshCw className="shrink-0 animate-spin text-primary" size={16} aria-hidden="true" />
        <span className="truncate">{props.message}</span>
        <span className="text-muted-foreground ml-auto shrink-0 tabular-nums">0%</span>
      </div>
      <Progress value={0} aria-label={`${props.message}进度`} />
    </div>
  );
}
