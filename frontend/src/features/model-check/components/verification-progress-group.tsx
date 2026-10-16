import { useId, useState, type ReactElement } from "react";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { verificationProbeKey, type VerificationRecord } from "../lib/verification-progress";
import { VerificationProbeProgress } from "./verification-probe-progress";

export function VerificationProgressGroup(props: {
  label: string;
  probes: VerificationRecord[];
  timeout: number | null;
  defaultExpanded?: boolean;
  showFullDetails?: boolean;
}): ReactElement {
  const [expanded, setExpanded] = useState(props.defaultExpanded ?? false);
  const id = useId();
  const completed = props.probes.filter((probe) =>
    ["passed", "observed", "failed", "error"].includes(String(probe.status)),
  ).length;
  const failed = props.probes.filter((probe) =>
    ["failed", "error"].includes(String(probe.status)),
  ).length;
  const running = props.probes.some((probe) => probe.status === "running");
  return (
    <section aria-label={props.label} className="min-w-0 overflow-hidden rounded-md border">
      <div className="bg-muted/10 px-2 py-1.5">
        <Button
          variant="ghost"
          className="w-full min-w-0 justify-start gap-2 px-1"
          aria-expanded={expanded}
          aria-controls={id}
          onClick={() => setExpanded(!expanded)}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn("shrink-0 transition-transform", expanded && "rotate-180")}
          />
          <span className="min-w-0 flex-1 truncate text-left">{props.label}</span>
          {running ? <span className="text-primary shrink-0 text-xs">检测中</span> : null}
          {failed > 0 ? (
            <span className="text-destructive shrink-0 text-xs">异常 {failed}</span>
          ) : null}
          <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
            {completed}/{props.probes.length} 项
          </span>
        </Button>
      </div>
      <div id={id} hidden={!expanded} className="min-w-0 divide-y border-t">
        {expanded
          ? props.probes.map((probe) => (
              <VerificationProbeProgress
                key={verificationProbeKey(probe)}
                probe={probe}
                timeout={props.timeout}
                compact
                showFullDetails={props.showFullDetails}
              />
            ))
          : null}
      </div>
    </section>
  );
}
