import { CheckCircle2, LoaderCircle, XCircle } from "lucide-react";
import { useState, type ReactElement } from "react";
import type { Task } from "@/api";
import { TaskCancelButton, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";
import {
  verificationNumber as number,
  verificationRecord as record,
  verificationRows as rows,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationCombinationProgress } from "./verification-combination-progress";

type Combination = { result: VerificationRecord; active: boolean };
function combinationKey(item: Combination): string {
  return JSON.stringify([text(item.result.account_id), text(item.result.claimed_model)]);
}

function CombinationButton(props: {
  item: Combination;
  selected: boolean;
  onSelect: () => void;
}): ReactElement {
  const result = props.item.result;
  const failed =
    Boolean(text(result.error)) ||
    rows(result.probes).some((probe) => probe.status === "failed" || probe.status === "error");
  let Icon = CheckCircle2;
  if (props.item.active) Icon = LoaderCircle;
  else if (failed) Icon = XCircle;
  return (
    <Button
      variant="ghost"
      aria-pressed={props.selected}
      onClick={props.onSelect}
      className="h-auto w-52 shrink-0 justify-start items-start gap-2 whitespace-normal p-3 text-left aria-pressed:bg-primary/10 aria-pressed:text-primary md:w-full"
    >
      <Icon
        aria-hidden="true"
        className={cn(
          "mt-0.5 size-4 shrink-0",
          props.item.active && "animate-spin",
          failed && "text-destructive",
        )}
      />
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 wrap-anywhere">{text(result.account_name) || "账号"}</span>
        <span className="mt-1 line-clamp-2 text-xs font-normal text-muted-foreground wrap-anywhere">
          {text(result.claimed_model) || "模型"}
        </span>
        <span className="mt-1 block text-xs font-normal text-muted-foreground">
          {props.item.active ? "检测中" : "已结束"} · 已执行 {rows(result.probes).length} 项
        </span>
      </span>
    </Button>
  );
}

export function LiveModelCheckResult(props: {
  task: Task;
  rows: VerificationRecord[];
}): ReactElement {
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const task = props.task;
  const completed = number(task.result.completed) ?? props.rows.length;
  const total = number(task.result.total);
  const phase = text(task.result.phase);
  const reportedProgress = number(task.progress);
  const progress = reportedProgress === null ? null : Math.min(100, Math.max(0, reportedProgress));
  const activeItems = rows(Object.values(record(task.result.active)));
  const queued = rows(task.result.queued);
  const combinations: Combination[] = [
    ...activeItems.map((result) => ({ result, active: true })),
    ...props.rows.map((result) => ({ result, active: false })),
  ];
  const selected =
    combinations.find((item) => combinationKey(item) === selectedKey) ?? combinations[0];
  let preparation = "等待检测任务开始…";
  if (phase === "credentials") preparation = "正在准备账号凭据，单项检测开始后将在此显示。";
  else if (phase === "testing") preparation = "等待单项检测进度，收到后会显示题目与回答。";
  return (
    <div
      className="flex h-full min-h-0 min-w-0 flex-col gap-3"
      data-testid="model-check-live-progress"
    >
      <header className="shrink-0 space-y-3 rounded-lg border bg-muted/20 p-3 sm:p-4">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h2 className="font-medium">实时检测过程</h2>
            <p className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground tabular-nums">
              <span className="whitespace-nowrap">
                已完成 {completed}/{total ?? "—"} 个组合
              </span>
              <span className="whitespace-nowrap">检测中 {activeItems.length}</span>
              <span className="whitespace-nowrap">等待 {queued.length}</span>
            </p>
          </div>
          <TaskCancelButton taskId={task.id} className="shrink-0" />
        </div>
        {progress !== null ? (
          <div className="flex items-center gap-3">
            <Progress
              value={progress}
              className="flex-1"
              aria-label={`检测进度 ${progress.toFixed(0)}%`}
            />
            <strong className="text-sm tabular-nums">{progress.toFixed(0)}%</strong>
          </div>
        ) : (
          <TaskStartupState message="正在准备检测" />
        )}
        <p role="status" aria-live="polite" className="text-muted-foreground text-xs wrap-anywhere">
          {task.message}
        </p>
      </header>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border md:flex-row">
        <nav
          aria-label="检测组合"
          className="flex max-h-32 min-h-0 shrink-0 gap-1 overflow-auto border-b bg-muted/20 p-2 md:block md:max-h-none md:w-60 md:space-y-1 md:border-r md:border-b-0"
        >
          <h3 className="sr-only px-2 py-1 text-xs font-medium text-muted-foreground md:not-sr-only">
            检测组合
          </h3>
          {combinations.map((item) => (
            <CombinationButton
              key={combinationKey(item)}
              item={item}
              selected={item === selected}
              onSelect={() => setSelectedKey(combinationKey(item))}
            />
          ))}
          {combinations.length === 0 ? (
            <p className="p-2 text-xs text-muted-foreground">准备检测组合…</p>
          ) : null}
          {queued.length > 0 ? (
            <details className="shrink-0 p-2 md:mt-2 md:border-t">
              <summary className="cursor-pointer text-xs text-muted-foreground">
                等待执行 · {queued.length} 个组合
              </summary>
              <ul className="mt-2 space-y-2 text-xs text-muted-foreground">
                {queued.map((item) => (
                  <li
                    key={JSON.stringify([item.account_id, item.claimed_model])}
                    className="space-y-1"
                  >
                    <span className="block wrap-anywhere">{text(item.account_name)}</span>
                    <span className="block wrap-anywhere">{text(item.claimed_model)}</span>
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </nav>
        <div
          role="region"
          aria-label="逐项检测过程"
          tabIndex={0}
          className="min-h-0 min-w-0 flex-1 overflow-auto p-3 sm:p-4"
        >
          {selected ? (
            <VerificationCombinationProgress
              key={combinationKey(selected)}
              result={selected.result}
              active={selected.active}
            />
          ) : (
            <TaskStartupState message={preparation} />
          )}
        </div>
      </div>
    </div>
  );
}
