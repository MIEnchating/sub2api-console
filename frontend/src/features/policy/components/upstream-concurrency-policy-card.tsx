import type { ReactElement } from "react";

import { upstreamConcurrencyPolicyLabels } from "../constants";
import { PolicyConfigCard } from "./policy-config-card";

export function UpstreamConcurrencyPolicyCard(props: {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}): ReactElement {
  return (
    <PolicyConfigCard
      title={upstreamConcurrencyPolicyLabels.title}
      description={upstreamConcurrencyPolicyLabels.description}
      switchAction={{
        checked: props.enabled,
        label: upstreamConcurrencyPolicyLabels.toggle,
        onCheckedChange: props.onEnabledChange,
      }}
    >
      <div className="text-muted-foreground col-span-full min-w-0 space-y-2 text-xs leading-5">
        <p>{upstreamConcurrencyPolicyLabels.scope}</p>
        <p>{upstreamConcurrencyPolicyLabels.recovery}</p>
      </div>
    </PolicyConfigCard>
  );
}
