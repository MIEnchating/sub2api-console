import type { ReactElement } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { AccountRow } from "../lib/detection-task-results";
import { terminalVerdicts, precheckVerdictLabels } from "../constants";
import { AnimationPreview } from "./animation-preview";
import { AnimationResultDetails } from "./animation-result-details";
import { PrecheckResultDetails } from "./precheck-result-details";
import { TerminalRoundResults } from "./terminal-round-results";
export function DetectionResultCard(props: {
  row: AccountRow;
  active: boolean;
  precheck: boolean;
  terminal: boolean;
}): ReactElement {
  const row = props.row;
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
          <div className="flex items-center gap-2 text-sm">
            <span className="min-w-0 flex-1">
              前置检测 · {precheckVerdictLabels[row.precheck.precheck?.verdict ?? "error"]}
            </span>
            <PrecheckResultDetails result={row.precheck} />
          </div>
        )}
        {row.terminal && (
          <div className="space-y-2 text-sm">
            <div>终端检测 · {terminalVerdicts[row.terminal.verdict].label}</div>
            <TerminalRoundResults rounds={row.terminal.round_results} />
          </div>
        )}
        {row.animation && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span>动画检测 · {row.animation.status === "succeeded" ? "成功" : "失败"}</span>
              <AnimationResultDetails result={row.animation} />
            </div>
            <div
              role="group"
              aria-label="动画预览区域"
              className="h-[180px] overflow-hidden rounded-md bg-muted/20"
            >
              {row.animation.status === "succeeded" && row.animation.svg ? (
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
          </div>
        )}
        {props.precheck && !row.precheck && (
          <p className="text-sm text-muted-foreground">
            前置检测 · {pendingStageLabel(props.active)}
          </p>
        )}
        {props.terminal && !row.terminal && (
          <p className="text-sm text-muted-foreground">
            终端检测 · {pendingStageLabel(props.active)}
          </p>
        )}
        {!row.animation && (
          <p className="text-sm text-muted-foreground">
            动画检测 · {pendingStageLabel(props.active)}
          </p>
        )}
      </div>
    </article>
  );
}

function pendingStageLabel(active: boolean): string {
  return active ? "等待检测结果" : "未返回结果";
}
