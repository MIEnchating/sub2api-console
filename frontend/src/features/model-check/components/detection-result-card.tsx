import { AnimationResultMetrics } from "./animation-result-metrics";
import { PrecheckResultMetrics } from "./precheck-result-metrics";
import type { ReactElement } from "react";
import type { Task } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { AccountRow } from "../lib/detection-task-results";
import {
  terminalVerdicts,
  precheckVerdictLabels,
  precheckVerdictTones,
  terminalVerdictTones,
  animationResultStatuses,
} from "../constants";
import { DetectionStageStatus } from "./detection-stage-status";
import { AnimationPreview } from "./animation-preview";
import { AnimationResultDetails } from "./animation-result-details";
import { PrecheckResultDetails } from "./precheck-result-details";
import { TerminalRoundResults } from "./terminal-round-results";
import { detectionMissingResult } from "../lib/detection-task-outcome";
export function DetectionResultCard(props: {
  row: AccountRow;
  active: boolean;
  animation?: boolean;
  precheck: boolean;
  terminal: boolean;
  task?: Task;
}): ReactElement {
  const row = props.row;
  const missing = detectionMissingResult(props.task, props.active);
  const hasMissingResult =
    (props.precheck && !row.precheck) ||
    (props.terminal && !row.terminal) ||
    (props.animation !== false && !row.animation);
  return (
    <article
      aria-label={"检测账号 " + row.name}
      className="min-w-0 overflow-hidden rounded-lg border bg-card"
    >
      <header className="flex min-w-0 items-center justify-between gap-2 border-b px-3 py-2">
        <Tooltip>
          <TooltipTrigger render={<h4 tabIndex={0} className="min-w-0 truncate font-medium" />}>
            {row.name}
          </TooltipTrigger>
          <TooltipContent role="tooltip" aria-label="账号完整名称">
            {row.name}
          </TooltipContent>
        </Tooltip>
        <span className="shrink-0 text-xs text-muted-foreground">ID {row.id}</span>
      </header>
      <div className="space-y-3 p-3">
        {row.precheck && (
          <div className="flex items-center justify-between gap-2 text-sm">
            <DetectionStageStatus
              stage="前置检测"
              label={precheckVerdictLabels[row.precheck.precheck?.verdict ?? "error"]}
              tone={precheckVerdictTones[row.precheck.precheck?.verdict ?? "error"]}
            />
            <PrecheckResultDetails result={row.precheck} />
          </div>
        )}
        {row.precheck ? <PrecheckResultMetrics result={row.precheck} /> : null}
        {row.terminal && (
          <div className="space-y-2 text-sm">
            <DetectionStageStatus
              stage="终端检测"
              label={terminalVerdicts[row.terminal.verdict].label}
              tone={terminalVerdictTones[row.terminal.verdict]}
            />
            <TerminalRoundResults rounds={row.terminal.round_results} />
          </div>
        )}
        {row.animation && (
          <div className="space-y-2">
            <div className="flex items-center justify-between gap-2 text-sm">
              <DetectionStageStatus
                stage="动画检测"
                label={animationResultStatuses[row.animation.status].label}
                tone={animationResultStatuses[row.animation.status].tone}
              />
              <AnimationResultDetails result={row.animation} />
            </div>
            <div
              role="group"
              aria-label="动画预览区域"
              className="h-[180px] overflow-hidden rounded-md bg-muted/20"
            >
              {row.animation.status === "succeeded" && (row.animation.html || row.animation.svg) ? (
                <AnimationPreview result={row.animation} className="rounded-none ring-0" />
              ) : (
                <div className="flex h-full min-w-0 flex-col items-center justify-center gap-2 px-4 text-center">
                  <p className="text-sm font-medium">动画生成失败</p>
                  <p className="line-clamp-2 text-xs text-muted-foreground wrap-anywhere">
                    {row.animation.error || "未返回动画内容"}
                  </p>
                  <p className="text-xs text-muted-foreground">完整原因可点击动画详情查看</p>
                </div>
              )}
            </div>
            <AnimationResultMetrics result={row.animation} showUsage={false} />
          </div>
        )}
        {props.precheck && !row.precheck && (
          <DetectionStageStatus stage="前置检测" label={missing.label} tone={missing.tone} />
        )}
        {props.terminal && !row.terminal && (
          <DetectionStageStatus stage="终端检测" label={missing.label} tone={missing.tone} />
        )}
        {props.animation !== false && !row.animation && (
          <DetectionStageStatus stage="动画检测" label={missing.label} tone={missing.tone} />
        )}
        {hasMissingResult && missing.reason ? (
          <p className="text-xs text-muted-foreground wrap-anywhere">{missing.reason}</p>
        ) : null}
      </div>
    </article>
  );
}
