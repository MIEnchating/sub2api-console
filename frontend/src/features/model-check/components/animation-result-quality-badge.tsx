import { LoaderCircle } from "lucide-react";
import { animationQualityPending } from "../lib/animation-task-results";
import { animationQualityPendingLabel } from "../constants";
import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { api, type AnimationResult } from "@/api";
import { compareTimestamps } from "@/lib/time-order";
import { AnimationQualityBadge } from "./animation-quality-badge";
import { qualityRecheckActive } from "./animation-quality-recheck";

export function AnimationResultQualityBadge(props: { result: AnimationResult }): ReactElement {
  if (animationQualityPending(props.result)) {
    return (
      <span
        role="status"
        aria-label={animationQualityPendingLabel}
        aria-busy="true"
        className="inline-flex items-center gap-1.5 text-muted-foreground"
      >
        <LoaderCircle aria-hidden="true" className="size-3.5 shrink-0 motion-safe:animate-spin" />
        {animationQualityPendingLabel}
      </span>
    );
  }
  if (!props.result.source_task_id || !props.result.request_id) {
    return <AnimationQualityBadge prediction={props.result.quality} />;
  }
  return <ObservedQualityBadge result={props.result} taskID={props.result.source_task_id} />;
}

function ObservedQualityBadge(props: { result: AnimationResult; taskID: string }): ReactElement {
  // Observe the same resource as the quality panel. A thumbnail never starts a
  // new read until a known recheck is active; closing the panel keeps it polled.
  const review = useQuery({
    queryKey: ["animation-quality", props.taskID, props.result.account_id, props.result.request_id],
    queryFn: ({ signal }) =>
      api.animationQuality(props.taskID, props.result.account_id, props.result.request_id, signal),
    enabled: (query) => qualityRecheckActive(query.state.data?.recheck?.status),
    refetchInterval: (query) =>
      qualityRecheckActive(query.state.data?.recheck?.status) ? 1000 : false,
  });
  let prediction = review.data?.prediction ?? props.result.quality;
  const original = props.result.quality;
  if (
    original?.reviewed_at &&
    (!prediction?.reviewed_at ||
      compareTimestamps(original.reviewed_at, prediction.reviewed_at) > 0)
  ) {
    prediction = original;
  }
  const annotation = review.data?.annotation ?? props.result.quality_annotation;
  if (annotation && annotation.source !== "confirmed_auto") {
    return (
      <AnimationQualityBadge
        prediction={{
          ...prediction,
          status: annotation.label,
          score: prediction?.score ?? null,
          benchmark: prediction?.benchmark ?? "",
          reason: undefined,
        }}
        manual
      />
    );
  }
  return <AnimationQualityBadge prediction={prediction} />;
}
