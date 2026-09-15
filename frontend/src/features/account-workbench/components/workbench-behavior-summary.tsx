import { useState, type ReactElement } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { behaviorVerdictLabels } from "../constants";

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function percent(value: unknown): string {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 100
    ? `${value}%`
    : "-";
}

export function WorkbenchBehaviorSummary(props: {
  report: Record<string, unknown> | null;
}): ReactElement | null {
  const [expanded, setExpanded] = useState(false);
  if (typeof props.report?.verdict !== "string") return null;
  const requests = record(props.report.requests);
  const failed =
    requests.successful === 0 && typeof requests.total === "number" && requests.total > 0;
  const verdict = failed ? "ERROR" : props.report.verdict;
  const coverage = record(props.report.coverage).percent ?? props.report.coverage_percent;
  const similarities = record(props.report.similarity_percent);
  const Icon = expanded ? ChevronDown : ChevronRight;
  return (
    <div className="min-w-0 text-sm">
      <button
        type="button"
        aria-expanded={expanded}
        className="flex min-h-8 items-center gap-1 text-left font-medium"
        onClick={() => setExpanded(!expanded)}
      >
        <Icon className="size-4" aria-hidden="true" />
        {behaviorVerdictLabels[verdict] ?? verdict}
      </button>
      {!failed && expanded && (
        <div className="grid gap-1 pt-1 text-xs text-muted-foreground">
          <p>有效覆盖 {percent(coverage)}</p>
          {Object.entries(similarities).map(([name, value]) => (
            <p key={name} className="wrap-anywhere">
              {name} {percent(value)}
            </p>
          ))}
        </div>
      )}
    </div>
  );
}
