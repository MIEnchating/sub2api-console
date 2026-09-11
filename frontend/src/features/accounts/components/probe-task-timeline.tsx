import { CheckCircle2, Circle, LoaderCircle, MinusCircle, XCircle } from "lucide-react";
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
            <Icon
              aria-hidden="true"
              className={`size-4 shrink-0 ${step.status === "running" ? "animate-spin motion-reduce:animate-none" : ""}`}
            />
            <span className="min-w-0 flex-1 break-words">
              {stageLabels[step.stage] ?? step.stage}
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
