import { OnboardingGroupBindingSelect } from "./onboarding-group-binding-select";

export function UpstreamGroupBindingEditor(props: {
  upstreamGroupName: string;
  upstreamPlatform: string | null;
  groups: Array<{
    id: string | null;
    name: string;
    platform?: string | null;
    platforms?: string[];
    rate_multiplier?: string | null;
  }>;
  value: string[];
  disabled: boolean;
  disabledReason: string | null;
  onValueChange: (value: string[]) => void;
}) {
  return (
    <OnboardingGroupBindingSelect
      upstreamGroupName={props.upstreamGroupName}
      upstreamPlatform={props.upstreamPlatform}
      groups={props.groups}
      value={props.value}
      disabled={props.disabled}
      disabledReason={props.disabledReason}
      onValueChange={props.onValueChange}
    />
  );
}
