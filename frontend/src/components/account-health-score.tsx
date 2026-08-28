import { cn } from "@/lib/utils";

export type AccountHealthScoreProps = {
  score: number | null;
  shortScore: number | null;
  longScore: number | null;
  sampleCount: number;
  className?: string;
};

function metric(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return "—";
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

export function AccountHealthScore(props: AccountHealthScoreProps) {
  const hasSamples = props.sampleCount > 0 && props.score !== null && Number.isFinite(props.score);
  const clampedScore = hasSamples ? Math.min(100, Math.max(0, props.score ?? 0)) : 0;
  const radius = 15.5;
  const circumference = 2 * Math.PI * radius;
  let textTone = "text-muted-foreground";
  let strokeTone = "stroke-muted-foreground/35";

  if (hasSamples && clampedScore >= 85) {
    textTone = "text-success";
    strokeTone = "stroke-success";
  } else if (hasSamples && clampedScore >= 60) {
    textTone = "text-warning";
    strokeTone = "stroke-warning";
  } else if (hasSamples) {
    textTone = "text-destructive";
    strokeTone = "stroke-destructive";
  }

  return (
    <div
      data-slot="account-health-score"
      className={cn("flex shrink-0 items-center gap-2 tabular-nums", props.className)}
    >
      <div
        className="relative size-9 shrink-0"
        aria-label={hasSamples ? `健康分 ${metric(props.score)}` : "暂无健康分"}
      >
        <svg viewBox="0 0 36 36" className="size-9 -rotate-90" aria-hidden="true">
          <circle
            cx="18"
            cy="18"
            r={radius}
            fill="none"
            strokeWidth="3"
            className="stroke-border"
          />
          <circle
            cx="18"
            cy="18"
            r={radius}
            fill="none"
            strokeWidth="3"
            strokeLinecap="round"
            className={strokeTone}
            strokeDasharray={circumference}
            strokeDashoffset={circumference * (1 - clampedScore / 100)}
          />
        </svg>
        <strong
          className={cn(
            "absolute inset-0 flex items-center justify-center text-[11px] font-semibold",
            textTone,
          )}
        >
          {hasSamples ? Math.round(clampedScore) : "—"}
        </strong>
      </div>
      <div className="text-muted-foreground grid text-xs">
        <span>短期 {hasSamples ? metric(props.shortScore) : "—"}</span>
        <span>长期 {hasSamples ? metric(props.longScore) : "—"}</span>
      </div>
    </div>
  );
}
