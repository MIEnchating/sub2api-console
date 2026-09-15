import type { ReactElement } from "react";

import type { UpstreamSummary } from "@/api";
import { FieldHelpTooltip } from "@/components/field-help-tooltip";
import { cn } from "@/lib/utils";

import { upstreamConcurrencyLabels } from "../constants";

type Props = {
  limit?: number | null;
  status?: UpstreamSummary["hosts"][number]["concurrency_status"];
  allocated?: number | null;
  target?: number | null;
};

function concurrencyLimitLabel(props: Props): string {
  if (props.status === "unknown" || props.limit == null) {
    return upstreamConcurrencyLabels.unknown;
  }
  if (props.limit === 0) return upstreamConcurrencyLabels.unlimited;
  return String(props.limit);
}

export function UpstreamConcurrencyHeading(): ReactElement {
  return (
    <span className="inline-flex items-center gap-1">
      {upstreamConcurrencyLabels.heading}
      <FieldHelpTooltip label={upstreamConcurrencyLabels.name}>
        {upstreamConcurrencyLabels.help}
      </FieldHelpTooltip>
    </span>
  );
}

function concurrencyExceeded(props: Props): boolean {
  return (
    props.status !== "unknown" &&
    props.limit != null &&
    props.limit > 0 &&
    props.allocated != null &&
    props.allocated > props.limit
  );
}

export function UpstreamConcurrency(props: Props): ReactElement {
  const unread = props.status === "unknown" || props.limit == null;
  const exceeded = concurrencyExceeded(props);
  return (
    <div
      role="group"
      aria-label={upstreamConcurrencyLabels.name}
      className="grid min-w-0 gap-0.5 whitespace-normal"
    >
      {unread ? (
        <span
          aria-label={upstreamConcurrencyLabels.limit}
          className="text-muted-foreground text-xs"
        >
          {upstreamConcurrencyLabels.unknown}
        </span>
      ) : (
        <div
          aria-label={upstreamConcurrencyLabels.ratio}
          className="flex min-w-0 flex-wrap items-baseline gap-x-1 text-sm tabular-nums"
        >
          <span
            aria-label={upstreamConcurrencyLabels.allocatedName}
            className={cn("min-w-0 font-medium break-all", exceeded && "text-destructive")}
          >
            {props.allocated ?? upstreamConcurrencyLabels.unknownAllocation}
          </span>
          <span aria-hidden="true" className="text-muted-foreground">
            {" / "}
          </span>
          <span
            aria-label={upstreamConcurrencyLabels.limit}
            className="text-muted-foreground min-w-0 break-all"
          >
            {concurrencyLimitLabel(props)}
          </span>
        </div>
      )}
      {!unread && props.target != null && props.target !== props.allocated ? (
        <div className="text-muted-foreground flex flex-wrap gap-x-1 text-xs">
          <span>{upstreamConcurrencyLabels.target}</span>
          <span
            aria-label={upstreamConcurrencyLabels.targetName}
            className="min-w-0 break-all tabular-nums"
          >
            {props.target}
          </span>
        </div>
      ) : null}
      {!unread ? <ConcurrencyStatus stale={props.status === "stale"} exceeded={exceeded} /> : null}
    </div>
  );
}

function ConcurrencyStatus(props: { stale: boolean; exceeded: boolean }): ReactElement | null {
  if (!props.stale && !props.exceeded) return null;
  return (
    <div className="flex flex-wrap gap-x-2 text-xs">
      {props.stale ? (
        <span className="text-muted-foreground">{upstreamConcurrencyLabels.stale}</span>
      ) : null}
      {props.exceeded ? (
        <span className="text-destructive">{upstreamConcurrencyLabels.exceeded}</span>
      ) : null}
    </div>
  );
}

export function UpstreamConcurrencyDetails(props: Props): ReactElement {
  return (
    <section
      aria-labelledby="upstream-concurrency-title"
      className="bg-muted/25 flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-1.5 rounded-md px-3 py-2"
    >
      <h3
        id="upstream-concurrency-title"
        className="text-muted-foreground inline-flex items-center gap-1 text-xs"
      >
        {upstreamConcurrencyLabels.detailTitle}
        <FieldHelpTooltip label={upstreamConcurrencyLabels.name}>
          {upstreamConcurrencyLabels.help}
        </FieldHelpTooltip>
      </h3>
      <UpstreamConcurrency {...props} />
    </section>
  );
}
