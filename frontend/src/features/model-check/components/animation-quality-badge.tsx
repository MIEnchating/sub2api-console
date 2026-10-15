import type { ReactElement } from "react";
import { CircleCheck, CircleHelp, CircleAlert, CircleX } from "lucide-react";
import type { AnimationQualityPrediction } from "@/api";
import { animationQualityFailureLabel } from "../lib/animation-quality-status";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  animationQualityLabels,
  animationQualityReasons,
  animationQualityVerdicts,
} from "../constants";

export function AnimationQualityBadge(props: {
  prediction?: AnimationQualityPrediction;
  manual?: boolean;
}): ReactElement {
  const prediction = props.prediction;
  if (!prediction) return <span className="text-muted-foreground">质量：未检测</span>;
  const failureLabel = props.manual ? undefined : animationQualityFailureLabel(prediction);
  const label = failureLabel ?? animationQualityLabels[prediction.status];
  const Icon = failureLabel
    ? CircleX
    : { normal: CircleCheck, suspicious: CircleAlert, unknown: CircleHelp }[prediction.status];
  const color = failureLabel
    ? "text-destructive"
    : {
        normal: "text-emerald-700 dark:text-emerald-400",
        suspicious: "text-destructive",
        unknown: "text-amber-800 dark:text-amber-300",
      }[prediction.status];
  const reason = props.manual ? undefined : animationQualityReasons[prediction.reason ?? ""];
  const badge = (
    <span
      className={`inline-flex max-w-full items-center gap-1 ${color}`}
      aria-label={`${props.manual ? "质量标注" : "自动质量"}：${label}`}
      aria-description={
        failureLabel ? "未获得有效质量结论" : animationQualityVerdicts[prediction.status]
      }
      tabIndex={reason ? 0 : undefined}
    >
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{label}</span>
    </span>
  );
  if (!reason) return badge;
  return (
    <Tooltip>
      <TooltipTrigger render={badge} />
      <TooltipContent role="tooltip">{reason}</TooltipContent>
    </Tooltip>
  );
}
