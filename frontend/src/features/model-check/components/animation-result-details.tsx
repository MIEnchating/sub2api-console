import { useRef, useState, type ReactElement } from "react";
import { FileText } from "lucide-react";
import type { AnimationResult } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

import { DetectionResultSummary, DetectionResultMetadata } from "./detection-result-summary";
import { DetectionStageStatus } from "./detection-stage-status";
import { animationResultStatuses } from "../constants";

export function AnimationResultDetails(props: { result: AnimationResult }): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const result = props.result;
  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              ref={triggerRef}
              type="button"
              variant="ghost"
              size="icon"
              aria-label="查看动画检测详情"
              aria-haspopup="dialog"
              aria-expanded={open}
              onClick={() => setOpen(true)}
            />
          }
        >
          <FileText aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>查看动画检测详情</TooltipContent>
      </Tooltip>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          width="wide"
          height="adaptive"
          finalFocus={triggerRef}
          className="sm:max-w-3xl"
        >
          <DialogHeader>
            <div className="flex flex-wrap items-center gap-2.5">
              <DialogTitle>动画检测详情</DialogTitle>
              <DetectionStageStatus
                stage="动画检测"
                label={animationResultStatuses[result.status].label}
                tone={animationResultStatuses[result.status].tone}
              />
            </div>
            <DialogDescription>{result.account_name}</DialogDescription>
          </DialogHeader>
          <DialogBody role="region" aria-label="动画检测详细结果" className="space-y-5">
            <DetectionResultSummary result={result} label="动画生成统计" />
            {result.error ? (
              <section
                aria-label="失败原因"
                className="space-y-2 rounded-lg border border-destructive/20 bg-destructive/5 p-3"
              >
                <h3 className="text-sm font-medium text-destructive">失败原因</h3>
                <p className="whitespace-pre-wrap text-sm leading-relaxed wrap-anywhere">
                  {result.error}
                </p>
              </section>
            ) : null}
            {result.status === "failed" ? (
              <section aria-label="生成原文" className="min-w-0 space-y-3">
                <h3 className="text-sm font-medium">生成原文</h3>
                {result.source ? (
                  <>
                    <pre
                      role="region"
                      aria-label="生成原文内容"
                      tabIndex={0}
                      className="max-h-[min(50svh,28rem)] overflow-auto overscroll-contain whitespace-pre-wrap rounded-md border bg-muted/30 p-3 font-mono text-xs leading-5 wrap-anywhere focus-visible:outline-2 focus-visible:outline-ring"
                    >
                      {result.source}
                    </pre>
                    {result.source_truncated ? (
                      <p className="text-xs text-muted-foreground">
                        原文超过 128 KB，仅展示前 128 KB。
                      </p>
                    ) : null}
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">本次结果未保存生成原文。</p>
                )}
              </section>
            ) : null}
            <DetectionResultMetadata result={result} />
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
