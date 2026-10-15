import type { ReactElement } from "react";
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, LoaderCircle } from "lucide-react";
import { toast } from "sonner";
import { api, type AnimationQualityReview, type AnimationResult } from "@/api";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";
import { AnimationQualityActionButton } from "./animation-quality-action-button";
import { qualityRecheckActive } from "./animation-quality-recheck";

export function AnimationQualityConfirm(props: {
  result: AnimationResult;
  taskID: string;
  review?: AnimationQualityReview;
  compact?: boolean;
}): ReactElement {
  const client = useQueryClient();
  const key = ["animation-quality", props.taskID, props.result.account_id, props.result.request_id];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      api.animationQuality(props.taskID, props.result.account_id, props.result.request_id, signal),
    enabled: false,
    retry: false,
  });
  const review = props.review ?? query.data;
  const prediction = review?.prediction ?? props.result.quality;
  const mutationKey = [...key, "confirm"];
  const saving = useIsMutating({ mutationKey }) > 0;
  const rechecking = useIsMutating({ mutationKey: [...key, "recheck"] }) > 0;
  const annotation = review?.annotation ?? props.result.quality_annotation;
  const fingerprint = review?.prediction_sha256 ?? props.result.quality_prediction_sha256;
  const collected =
    annotation?.source === "confirmed_auto" &&
    Boolean(fingerprint) &&
    annotation.prediction_sha256 === fingerprint;
  const manuallyAnnotated = Boolean(annotation && annotation.source !== "confirmed_auto");
  const eligible =
    (props.compact || Boolean(review?.prediction_sha256)) &&
    prediction?.source === "model_review" &&
    (prediction.status === "normal" || prediction.status === "suspicious") &&
    Boolean(prediction.explanation?.trim()) &&
    !prediction.reason &&
    !qualityRecheckActive(review?.recheck?.status) &&
    !rechecking;
  const save = useMutation({
    retry: false,
    mutationKey,
    mutationFn: async () => {
      let current = review;
      if (props.compact) {
        const latest = await query.refetch();
        if (latest.isError || !latest.data) return null;
        current = latest.data;
        if (
          current.prediction.status !== prediction?.status ||
          current.prediction.explanation !== prediction?.explanation ||
          current.prediction.reviewed_at !== prediction?.reviewed_at
        ) {
          throw new Error("评审结果已变化，请核对更新后的质量标注再确认收录");
        }
      }
      if (!current) return null;
      if (props.compact && current.annotation && current.annotation.source !== "confirmed_auto") {
        throw new Error("该结果已有人工纠正，请核对质量标注后再更新样本");
      }
      return api.confirmAnimationQuality(props.taskID, {
        account_id: props.result.account_id,
        request_id: props.result.request_id,
        version: current.annotation?.version ?? 0,
        recheck_version: current.recheck_version ?? 0,
        prediction_sha256: current.prediction_sha256 ?? "",
      });
    },
    onSuccess: (saved) => {
      if (!saved) return;
      client.setQueryData(
        ["animation-quality", props.taskID, props.result.account_id, props.result.request_id],
        saved,
      );
      void client.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
      toast.success("已收录训练样本，可在页面顶部导出全部样本");
    },
    onError: (error) => notifyOperationError(error, "训练样本收录失败"),
  });
  let label = "确认结果并收录样本";
  if (saving) label = "正在收录";
  else if (collected) label = "已收录训练样本";
  else if (annotation) label = "使用自动结果更新样本";
  if (props.compact) {
    let compactLabel = "确认收录";
    if (saving) compactLabel = "正在收录";
    else if (collected) compactLabel = "已收录";
    else if (manuallyAnnotated) compactLabel = "已标注";
    else if (annotation) compactLabel = "更新收录";
    return (
      <AnimationQualityActionButton
        label={compactLabel}
        disabled={!eligible || collected || saving || manuallyAnnotated}
        busy={saving}
        onClick={() => save.mutate()}
      >
        {saving ? (
          <LoaderCircle className="animate-spin" aria-hidden="true" />
        ) : (
          <Check aria-hidden="true" />
        )}
      </AnimationQualityActionButton>
    );
  }
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <p className="min-w-0 text-xs leading-5 text-muted-foreground" role="status">
        {collected
          ? "自动结果已确认，标签和理由已保存。"
          : "核对自动结论后，一并收录标签、理由与原文。"}
      </p>
      <Button
        className="w-full sm:w-auto"
        disabled={!eligible || collected || saving}
        aria-busy={saving}
        onClick={() => save.mutate()}
      >
        {saving ? (
          <LoaderCircle className="animate-spin" aria-hidden="true" />
        ) : (
          <Check aria-hidden="true" />
        )}
        {label}
      </Button>
    </div>
  );
}
