import { OnboardingGroupBindingSelect } from "./onboarding-group-binding-select";

export function UpstreamGroupBindingEditor(props: {
  upstreamGroupName: string;
  groups: Array<{ id: string | null; name: string; rate_multiplier?: string | null }>;
  value: string[];
  disabled: boolean;
  disabledReason: string | null;
  onValueChange: (value: string[]) => void;
}) {
  return (
    <OnboardingGroupBindingSelect
      upstreamGroupName={props.upstreamGroupName}
      groups={props.groups}
      value={props.value}
      disabled={props.disabled}
      disabledReason={props.disabledReason}
      onValueChange={props.onValueChange}
    />
  );
}
