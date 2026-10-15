import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  CircleHelp,
  LoaderCircle,
  XCircle,
} from "lucide-react";
import type { ReactElement } from "react";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

function stateTone(status: string): "success" | "warning" | "destructive" | "neutral" | "info" {
  if (
    [
      "passed",
      "observed",
      "stable",
      "official_source",
      "not_applicable",
      "matched",
      "reference_consistent",
    ].includes(status)
  )
    return "success";
  if (["failed", "error"].includes(status)) return "destructive";
  if (["unstable", "evidence_mismatch", "mismatch"].includes(status)) return "warning";
  if (status === "running") return "info";
  return "neutral";
}

export function VerificationStateIcon(props: { status: string }): ReactElement {
  const tone = stateTone(props.status);
  let Icon = CircleHelp;
  let color = "text-muted-foreground";
  if (tone === "success") {
    Icon = CheckCircle2;
    color = "text-success";
  } else if (tone === "destructive") {
    Icon = XCircle;
    color = "text-destructive";
  } else if (tone === "warning") {
    Icon = AlertTriangle;
    color = "text-warning";
  } else if (tone === "info") {
    Icon = LoaderCircle;
    color = "text-info animate-spin";
  } else if (["pending", "skipped", "not_run"].includes(props.status)) Icon = Circle;
  return <Icon aria-hidden="true" className={cn("size-4 shrink-0", color)} />;
}

export function VerificationStateBadge(props: { status: string; label: string }): ReactElement {
  return (
    <Tooltip>
      <TooltipTrigger
        render={<Badge variant={stateTone(props.status)} />}
        data-slot="badge"
        tabIndex={0}
      >
        <span className="inline-flex max-w-full min-w-0 items-center gap-1 align-middle">
          <VerificationStateIcon status={props.status} />
          <span className="min-w-0 truncate">{props.label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent role="tooltip">{props.label}</TooltipContent>
    </Tooltip>
  );
}
