import { PrecheckResultMetrics } from "./precheck-result-metrics";
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
import { Badge } from "@/components/ui/badge";
import { precheckQuestionLabels, precheckVerdictLabels, precheckVerdictTones } from "../constants";
import { AnimationEndpointDetails } from "./animation-endpoint-details";
import { DetectionStageStatus } from "./detection-stage-status";

export function PrecheckResultDetails(props: { result: AnimationResult }): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const check = props.result.precheck;
  const showTaskError =
    props.result.error &&
    !check?.questions.some((question) => question.error === props.result.error);

  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              ref={triggerRef}
              type="button"
              size="icon"
              variant="ghost"
              className="ml-auto shrink-0"
              aria-label="查看前置检测详情"
              aria-haspopup="dialog"
              aria-expanded={open}
              onClick={() => setOpen(true)}
            />
          }
        >
          <FileText aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>查看前置检测详情</TooltipContent>
      </Tooltip>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent width="wide" height="adaptive" finalFocus={triggerRef}>
          <DialogHeader>
            <div className="flex flex-wrap items-center gap-3 pr-8">
              <DialogTitle>前置检测详情</DialogTitle>
              <DetectionStageStatus
                stage="前置检测"
                label={precheckVerdictLabels[check?.verdict ?? "error"]}
                tone={precheckVerdictTones[check?.verdict ?? "error"]}
              />
            </div>
            <DialogDescription>{props.result.account_name}</DialogDescription>
            <PrecheckResultMetrics result={props.result} detailed />
          </DialogHeader>
          <DialogBody role="region" aria-label="前置检测详细结果" className="space-y-5">
            <section
              aria-label="模型回答"
              tabIndex={0}
              className="max-h-[50svh] min-h-0 space-y-4 overflow-y-auto overscroll-contain border-t pt-4 pr-2"
            >
              <h3 className="text-sm font-medium text-muted-foreground">模型回答</h3>
              {showTaskError ? (
                <p className="whitespace-pre-wrap text-destructive wrap-anywhere">
                  {props.result.error}
                </p>
              ) : null}
              {check?.questions.map((question) => (
                <section key={question.id} className="min-w-0 space-y-2">
                  <div className="flex min-w-0 items-start justify-between gap-3">
                    <h4 className="min-w-0 font-medium wrap-anywhere">
                      {precheckQuestionLabels[question.id] ?? question.id}
                    </h4>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {precheckVerdictLabels[question.verdict]}
                    </span>
                  </div>
                  {question.error ? (
                    <p className="whitespace-pre-wrap text-destructive wrap-anywhere">
                      {question.error}
                    </p>
                  ) : null}
                  {question.answer && question.answer !== question.error ? (
                    <p className="whitespace-pre-wrap leading-relaxed wrap-anywhere">
                      {question.answer}
                    </p>
                  ) : null}
                  {question.answer_truncated ? (
                    <p className="text-xs text-muted-foreground">
                      回答超过保存上限，已截断（最多 65,536 字符）
                    </p>
                  ) : null}
                  {!question.answer && !question.error ? (
                    <p className="text-muted-foreground">未返回回答</p>
                  ) : null}
                  {question.request_id ? (
                    <p className="text-xs text-muted-foreground wrap-anywhere">
                      请求 ID：<span className="font-mono">{question.request_id}</span>
                    </p>
                  ) : null}
                </section>
              ))}
              {!check?.questions.length && !showTaskError ? (
                <p className="text-muted-foreground">未返回回答</p>
              ) : null}
            </section>
            <dl className="grid grid-cols-[5rem_minmax(0,1fr)] gap-x-3 gap-y-2 border-t pt-4 text-xs">
              <AnimationEndpointDetails source={props.result} />
              <dt className="text-muted-foreground">检测模型</dt>
              <dd className="wrap-anywhere">{props.result.model}</dd>
              {props.result.response_model ? (
                <>
                  <dt className="text-muted-foreground">返回模型</dt>
                  <dd className="wrap-anywhere">{props.result.response_model}</dd>
                  {props.result.response_model !== props.result.model ? (
                    <>
                      <dt className="text-muted-foreground">模型状态</dt>
                      <dd>
                        <Badge variant="destructive">重点：模型不一致</Badge>
                      </dd>
                    </>
                  ) : null}
                </>
              ) : null}
              {props.result.request_id ? (
                <>
                  <dt className="text-muted-foreground">请求 ID</dt>
                  <dd className="font-mono wrap-anywhere">{props.result.request_id}</dd>
                </>
              ) : null}
            </dl>
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
