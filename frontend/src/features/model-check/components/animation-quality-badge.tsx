import type { ReactElement } from "react";
import { CircleCheck, CircleHelp, CircleAlert } from "lucide-react";
import type { AnimationQualityPrediction } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { animationQualityLabels, animationQualityReasons } from "../constants";

export function AnimationQualityBadge(props: {
  prediction?: AnimationQualityPrediction;
}): ReactElement {
  const prediction = props.prediction;
  if (!prediction) return <span className="text-muted-foreground">质量：未检测</span>;
  const Icon = { normal: CircleCheck, suspicious: CircleAlert, unknown: CircleHelp }[
    prediction.status
  ];
  const color = {
    normal: "text-emerald-700 dark:text-emerald-400",
    suspicious: "text-destructive",
    unknown: "text-amber-800 dark:text-amber-300",
  }[prediction.status];
  const reason = animationQualityReasons[prediction.reason ?? ""];
  const badge = (
    <span
      className={`inline-flex max-w-full items-center gap-1 ${color}`}
      aria-label={`自动质量：${animationQualityLabels[prediction.status]}`}
      tabIndex={reason ? 0 : undefined}
    >
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{animationQualityLabels[prediction.status]}</span>
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
