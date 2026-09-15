import type { ReactElement } from "react";

import { costWallPolicyLabels } from "../constants";
import { PolicyConfigCard } from "./policy-config-card";
import { PolicySwitchRow } from "./policy-switch-row";

export function CostWallPolicyCard(props: {
  enabled: boolean;
  fallbackEnabled: boolean;
  stopAutoProbe: boolean;
  onEnabledChange: (value: boolean) => void;
  onFallbackEnabledChange: (value: boolean) => void;
  onStopAutoProbeChange: (value: boolean) => void;
}): ReactElement {
  return (
    <PolicyConfigCard
      title={costWallPolicyLabels.title}
      description={costWallPolicyLabels.description}
      switchAction={{
        checked: props.enabled,
        label: costWallPolicyLabels.toggle,
        onCheckedChange: props.onEnabledChange,
      }}
    >
      <PolicySwitchRow
        label={costWallPolicyLabels.fallback}
        description={costWallPolicyLabels.fallbackDescription}
        checked={props.fallbackEnabled}
        disabled={!props.enabled}
        onCheckedChange={props.onFallbackEnabledChange}
      />
      <PolicySwitchRow
        label={costWallPolicyLabels.stopAutoProbe}
        description={costWallPolicyLabels.stopAutoProbeDescription}
        checked={props.stopAutoProbe}
        disabled={!props.enabled}
        onCheckedChange={props.onStopAutoProbeChange}
      />
    </PolicyConfigCard>
  );
}
