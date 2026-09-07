import type { ReactElement } from "react";
import { formatHealthScore } from "@/lib/health-score";
import { cn } from "@/lib/utils";

export type AccountHealthScoreProps = {
  score: number | null;
  shortScore: number | null;
  longScore: number | null;
  sampleCount: number;
  className?: string;
};

export function AccountHealthScore(props: AccountHealthScoreProps): ReactElement {
  const hasSamples = props.sampleCount > 0 && props.score !== null && Number.isFinite(props.score);
  const score = formatHealthScore(hasSamples ? props.score : null);
  const shortScore = formatHealthScore(hasSamples ? props.shortScore : null);
  const longScore = formatHealthScore(hasSamples ? props.longScore : null);
  const progress = hasSamples ? Math.min(100, Math.max(0, props.score ?? 0)) : 0;
  const circumference = 2 * Math.PI * 17.5;
  let tone = "text-muted-foreground";
  if (hasSamples && progress >= 85) tone = "text-success";
  else if (hasSamples && progress >= 60) tone = "text-warning";
  else if (hasSamples) tone = "text-destructive";

  return (
    <div
      data-slot="account-health-score"
      className={cn("grid w-fit shrink-0 gap-1.5 tabular-nums", props.className)}
    >
      <div className="flex items-center gap-2.5">
        <div
          className={cn("relative size-10 shrink-0", tone)}
          aria-label={hasSamples ? `健康分 ${score}` : "暂无健康分"}
        >
          <svg viewBox="0 0 40 40" className="size-10 -rotate-90" aria-hidden="true">
            <circle
              cx="20"
              cy="20"
              r="17.5"
              fill="none"
              strokeWidth="3"
              className="stroke-border"
            />
            <circle
              cx="20"
              cy="20"
              r="17.5"
              fill="none"
              strokeWidth="3"
              stroke="currentColor"
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={circumference * (1 - progress / 100)}
            />
          </svg>
          <strong className="absolute inset-0 flex items-center justify-center text-[11px]! font-semibold">
            {score}
          </strong>
        </div>
        <div className="grid gap-1 text-xs!">
          <span
            className="flex min-w-16 justify-between gap-2"
            aria-label={`短期评分 ${shortScore}`}
          >
            <span className="text-muted-foreground text-xs!">短期 </span>
            <span className="text-xs! font-medium">{shortScore}</span>
          </span>
          <span
            className="flex min-w-16 justify-between gap-2"
            aria-label={`长期评分 ${longScore}`}
          >
            <span className="text-muted-foreground text-xs!">长期 </span>
            <span className="text-xs! font-medium">{longScore}</span>
          </span>
        </div>
      </div>
      <span className="text-muted-foreground text-xs!">有效样本 {props.sampleCount}</span>
    </div>
  );
}
