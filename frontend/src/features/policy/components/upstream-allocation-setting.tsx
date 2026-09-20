import type { ReactElement } from "react";
import type { UpstreamAllocationTarget } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Switch } from "@/components/ui/switch";
import { FieldLabel } from "@/components/field-help-tooltip";
import { cn } from "@/lib/utils";
import { allocationSettingLabels as labels } from "../constants";
import type { UpstreamAllocationDraft } from "../hooks/use-upstream-allocation-draft";

export function UpstreamAllocationSetting(props: {
  kind: UpstreamAllocationTarget;
  id: string;
  draft: UpstreamAllocationDraft;
  disabled?: boolean;
  layout?: "card" | "row";
}): ReactElement {
  const query = props.draft.query;
  const data = query.data;
  if (!data) {
    return query.isError ? (
      <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />
    ) : (
      <ContentLoading label={labels.loading} compact />
    );
  }
  const id = `shared-allocation-${props.kind}-${props.id}`;
  const help = props.kind === "accounts" ? labels.accountHelp : labels.upstreamHelp;
  return (
    <section
      className={cn(
        "flex min-h-12 min-w-0 items-center justify-between gap-3 px-3 py-2",
        props.layout !== "row" && "rounded-lg border",
      )}
      aria-label={labels.title}
    >
      <FieldLabel
        htmlFor={id}
        label={labels.title}
        description={data.global_enabled ? help : labels.masterOff}
        className="text-sm"
      />
      <Switch
        id={id}
        checked={props.draft.checked}
        disabled={props.disabled || !data.global_enabled}
        className="shrink-0"
        onCheckedChange={props.draft.onCheckedChange}
      />
    </section>
  );
}
