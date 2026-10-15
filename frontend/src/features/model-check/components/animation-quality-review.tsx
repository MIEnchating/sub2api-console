import { useId, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, FileText, PencilLine, RefreshCw } from "lucide-react";
import { api, type AnimationResult } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { animationQualityFailureLabel } from "../lib/animation-quality-status";
import { animationQualityPending } from "../lib/animation-task-results";
import {
  animationQualityPendingLabel,
  animationQualityLabels,
  animationQualityReasons,
} from "../constants";
import { cn } from "@/lib/utils";
import { AnimationQualityActionButton } from "./animation-quality-action-button";
import { AnimationQualityForm } from "./animation-quality-form";
import { AnimationQualityBadge } from "./animation-quality-badge";
import { AnimationQualityBasis } from "./animation-quality-basis";
import { AnimationQualityConfirm } from "./animation-quality-confirm";
import { AnimationQualityRecheck, qualityRecheckActive } from "./animation-quality-recheck";
import { AnimationQualitySettingsDialog } from "./animation-quality-settings";

export function AnimationQualityReviewPanel(props: { result: AnimationResult }): ReactElement {
  const result = props.result;
  if (animationQualityPending(result))
    return <ContentLoading label={animationQualityPendingLabel} />;
  if (
    (result.source_task_status === "running" ||
      result.source_task_status === "queued" ||
      result.source_task_status === "waiting_input") &&
    result.phase !== "completed"
  ) {
    return <p className="text-sm text-muted-foreground">本项检测完成后可进行质量标注。</p>;
  }
  if (
    !result.source_task_id ||
    !result.request_id ||
    !result.source ||
    !result.prompt ||
    !result.html ||
    result.source_truncated ||
    result.status !== "succeeded" ||
    result.mode === "precheck" ||
    result.precheck
  ) {
    return (
      <p className="text-sm text-muted-foreground">
        该记录缺少完整 HTML、提示词或请求信息，无法作为质量标注样本。请重新检测后标注。
      </p>
    );
  }
  return <QualityReview result={result} taskID={result.source_task_id} />;
}
function QualityReview(props: { result: AnimationResult; taskID: string }): ReactElement {
  const [correcting, setCorrecting] = useState(false);
  const disclosureID = useId();
  const key = ["animation-quality", props.taskID, props.result.account_id, props.result.request_id];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      api.animationQuality(props.taskID, props.result.account_id, props.result.request_id, signal),
    staleTime: 30_000,
    refetchInterval: (query) =>
      qualityRecheckActive(query.state.data?.recheck?.status) ? 1000 : false,
  });
  if (!query.data) {
    if (query.isError)
      return <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />;
    return <ContentLoading label="正在读取质量标注" />;
  }
  const prediction = query.data.prediction;
  const failureLabel = animationQualityFailureLabel(prediction);
  let reasonTitle = failureLabel ? "失败原因" : "判断理由";
  if (prediction.reason === "review_cancelled") reasonTitle = "取消原因";
  return (
    <section aria-label="作品质量标注" className="flex h-full min-h-0 min-w-0 flex-col text-sm">
      <header className="shrink-0 space-y-3 border-b px-4 py-3 sm:px-5">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="font-semibold">自动质量标注</h3>
            <AnimationQualityBadge prediction={prediction} />
          </div>
          {prediction.source === "model_review" ? (
            <Tooltip>
              <TooltipTrigger
                render={<p className="min-w-0 max-w-full truncate text-xs text-muted-foreground" />}
                tabIndex={0}
              >
                模型自动评审 · {prediction.model_version}
              </TooltipTrigger>
              <TooltipContent role="tooltip">{prediction.model_version}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
        <div className="flex flex-wrap items-start gap-2">
          <div className="min-w-0 flex-1">
            <AnimationQualityRecheck
              result={props.result}
              taskID={props.taskID}
              review={query.data}
            />
          </div>
          <AnimationQualitySettingsDialog />
          <AnimationQualityActionButton
            label="重新读取标注"
            disabled={query.isFetching}
            busy={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" className={cn(query.isFetching && "animate-spin")} />
          </AnimationQualityActionButton>
        </div>
      </header>
      <div
        role="region"
        aria-label="标注内容"
        tabIndex={0}
        className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <div className="mx-auto max-w-4xl space-y-5 px-4 py-4 sm:px-5 sm:py-5">
          <section aria-label={reasonTitle} className="space-y-3">
            <h4 className="flex items-center gap-2 font-medium">
              <FileText className="size-4 text-muted-foreground" aria-hidden="true" />
              {reasonTitle}
            </h4>
            {prediction.explanation ? (
              <p className="whitespace-pre-wrap leading-7 wrap-anywhere">
                {prediction.explanation}
              </p>
            ) : null}
            {prediction.reason ? (
              <p className="rounded-lg bg-muted/50 px-3 py-2 leading-6 text-muted-foreground wrap-anywhere">
                {animationQualityReasons[prediction.reason] ?? prediction.reason}
              </p>
            ) : null}
            {!prediction.explanation && !prediction.reason ? (
              <p className="text-muted-foreground">暂无判断理由，可重新评审后查看。</p>
            ) : null}
            {prediction.score !== null && prediction.score !== undefined ? (
              <p className="text-xs text-muted-foreground">
                模型得分：{prediction.score.toFixed(4)} · {prediction.model_version}
              </p>
            ) : null}
          </section>
          <AnimationQualityBasis basis={prediction.basis} />
          <section aria-label="人工复核" className="border-t pt-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="space-y-1">
                <h4 className="font-medium">人工复核</h4>
                <p className="text-xs text-muted-foreground">结论有偏差时，再修改标签与理由。</p>
              </div>
              <Button
                variant="ghost"
                aria-expanded={correcting}
                aria-controls={`${disclosureID}-correction`}
                onClick={() => setCorrecting(!correcting)}
              >
                <PencilLine aria-hidden="true" />
                人工纠正
                <ChevronDown
                  aria-hidden="true"
                  className={cn("transition-transform", correcting && "rotate-180")}
                />
              </Button>
            </div>
            {query.data.annotation && !correcting ? (
              <p className="mt-3 text-xs text-muted-foreground">
                已标注：{animationQualityLabels[query.data.annotation.label]}
              </p>
            ) : null}
            {correcting ? (
              <div
                id={`${disclosureID}-correction`}
                className="mt-4 rounded-lg border bg-muted/20 p-3 sm:p-4"
              >
                <AnimationQualityForm
                  key={`${query.data.sample_id}:${query.data.annotation?.version ?? 0}`}
                  review={query.data}
                  result={props.result}
                  taskID={props.taskID}
                />
              </div>
            ) : null}
          </section>
        </div>
      </div>
      <footer aria-label="样本收录操作" className="shrink-0 border-t bg-muted/20 px-4 py-3 sm:px-5">
        <AnimationQualityConfirm result={props.result} taskID={props.taskID} review={query.data} />
      </footer>
    </section>
  );
}
