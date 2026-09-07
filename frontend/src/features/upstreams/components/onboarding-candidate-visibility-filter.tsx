import { Switch } from "@/components/ui/switch";

export function OnboardingCandidateVisibilityFilter(props: {
  onlyShowEnabled: boolean;
  hiddenCount: number;
  onOnlyShowEnabledChange: (checked: boolean) => void;
}) {
  return (
    <div
      className="flex shrink-0 flex-wrap items-center justify-end gap-x-2 gap-y-1"
      data-slot="onboarding-candidate-visibility-filter"
    >
      {props.onlyShowEnabled && props.hiddenCount > 0 ? (
        <span className="text-muted-foreground mr-1 text-xs" aria-live="polite">
          已隐藏 {props.hiddenCount} 个未启用分组
        </span>
      ) : null}
      <Switch
        id="onboarding-only-enabled-groups"
        checked={props.onlyShowEnabled}
        onCheckedChange={props.onOnlyShowEnabledChange}
      />
      <label className="cursor-pointer text-sm" htmlFor="onboarding-only-enabled-groups">
        仅显示启用分组
      </label>
    </div>
  );
}
