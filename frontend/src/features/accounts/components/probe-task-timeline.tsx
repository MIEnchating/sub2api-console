import { useId, useState, type ReactNode } from "react";
import {
  CheckCircle2,
  ChevronDown,
  Circle,
  LoaderCircle,
  MinusCircle,
  XCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { TaskStartupState } from "@/components/task-startup-state";
import type { ProbeStep } from "../hooks/use-onboarding-probe-task";

const stageLabels: Record<string, string> = {
  credential: "校验上游分组与鉴权",
  read_key: "读取已有上游 Key",
  create_key: "创建临时上游 Key",
  reuse_key: "复用已准备的临时 Key",
  models: "获取上游模型列表",
  request: "发送探活请求并等待响应",
  cleanup_key: "清理临时上游 Key",
  reconcile_key: "核对创建结果并清理临时 Key",
};
const statusLabels = {
  running: "进行中",
  succeeded: "已完成",
  failed: "失败",
  skipped: "无需清理",
};

export function ProbeProgressSummary(props: {
  steps: ProbeStep[];
  pendingMessage?: string;
  children?: ReactNode;
}) {
  const [expanded, setExpanded] = useState(false);
  const id = useId();
  const recentSteps = [...props.steps].reverse();
  const latest =
    recentSteps.find((step) => step.status === "running") ??
    recentSteps.find((step) => step.status === "failed") ??
    recentSteps[0];
  let stageLabel = "";
  if (latest)
    stageLabel = Object.hasOwn(stageLabels, latest.stage)
      ? stageLabels[latest.stage]
      : latest.stage;
  const showingDetails = expanded && Boolean(latest);
  let Icon = CheckCircle2;
  if (props.pendingMessage || latest?.status === "running") Icon = LoaderCircle;
  else if (latest?.status === "failed") Icon = XCircle;
  else if (latest?.status === "skipped") Icon = MinusCircle;
  return (
    <div className="grid min-w-0 gap-2">
      {latest ? (
        <Button
          variant="ghost"
          className="w-full min-w-0 justify-start gap-2 px-0"
          aria-expanded={showingDetails}
          aria-controls={id}
          onClick={() => setExpanded(!expanded)}
        >
          <span className="flex size-4 shrink-0 items-center justify-center overflow-hidden">
            <Icon
              aria-hidden="true"
              className={
                Icon === LoaderCircle ? "size-4 animate-spin motion-reduce:animate-none" : "size-4"
              }
            />
          </span>
          <span className="min-w-0 flex-1 truncate text-left">
            {props.pendingMessage || stageLabel}
          </span>
          <span className="text-muted-foreground shrink-0 text-xs">
            {props.pendingMessage ? "进行中" : statusLabels[latest.status]}
          </span>
          <span aria-hidden="true" className="text-muted-foreground shrink-0 text-xs">
            {showingDetails && props.children ? "返回响应" : "过程"}
          </span>
          <ChevronDown aria-hidden="true" className={showingDetails ? "rotate-180" : undefined} />
        </Button>
      ) : (
        <div className="min-h-8 min-w-0 overflow-hidden">
          {props.pendingMessage ? (
            <TaskStartupState message={props.pendingMessage} className="min-h-8 py-0 text-xs" />
          ) : null}
        </div>
      )}
      <div
        id={id}
        hidden={!showingDetails && !props.children}
        data-slot="probe-detail-panel"
        className={
          props.children
            ? "h-48 min-w-0 overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950"
            : "min-w-0"
        }
      >
        {showingDetails ? (
          <div
            className={
              props.children
                ? "h-full min-w-0 bg-popover overflow-y-auto overflow-x-hidden overscroll-contain p-3 [scrollbar-gutter:stable]"
                : "min-w-0"
            }
          >
            <ProbeTaskTimeline steps={props.steps} />
          </div>
        ) : (
          props.children
        )}
      </div>
    </div>
  );
}

export function ProbeTaskTimeline(props: { steps: ProbeStep[] }) {
  if (!props.steps.length) return null;
  return (
    <ol aria-label="探活过程" aria-live="polite" className="grid min-w-0 gap-2 text-xs">
      {props.steps.map((step, index) => {
        let Icon = Circle;
        let tone = "text-muted-foreground";
        if (step.status === "running") {
          Icon = LoaderCircle;
          tone = "text-foreground";
        }
        if (step.status === "succeeded") {
          Icon = CheckCircle2;
          tone = "text-emerald-600 dark:text-emerald-400";
        }
        if (step.status === "failed") {
          Icon = XCircle;
          tone = "text-destructive";
        }
        if (step.status === "skipped") Icon = MinusCircle;
        const elapsed = step.finished_at
          ? Date.parse(step.finished_at) - Date.parse(step.started_at)
          : null;
        return (
          <li key={`${index}:${step.stage}`} className={`flex min-w-0 items-start gap-2 ${tone}`}>
            <span className="flex size-4 shrink-0 items-center justify-center overflow-hidden">
              <Icon
                aria-hidden="true"
                className={`size-4 shrink-0 ${step.status === "running" ? "animate-spin motion-reduce:animate-none" : ""}`}
              />
            </span>
            <span className="min-w-0 flex-1 break-words">
              {Object.hasOwn(stageLabels, step.stage) ? stageLabels[step.stage] : step.stage}
            </span>
            <span className="shrink-0">
              {statusLabels[step.status]}
              {elapsed !== null && Number.isFinite(elapsed) ? ` · ${Math.max(0, elapsed)} ms` : ""}
            </span>
          </li>
        );
      })}
    </ol>
  );
}
