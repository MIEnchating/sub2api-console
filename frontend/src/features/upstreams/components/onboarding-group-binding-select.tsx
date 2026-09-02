import { MultiSelect } from "@/components/multi-select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { compatibleOnboardingLocalGroups } from "@/lib/onboarding-entry";

type BindingGroupOption = {
  id: string | null;
  name: string;
  platform?: string | null;
  platforms?: string[];
  rate_multiplier?: string | null;
};

export function OnboardingGroupBindingOption(props: { group: BindingGroupOption }) {
  return (
    <span className="flex min-w-0 flex-1 items-center justify-between gap-4">
      <span className="min-w-0 truncate">{props.group.name}</span>
      <span className="text-muted-foreground shrink-0 tabular-nums">
        {props.group.rate_multiplier?.trim() || "未设置"}
      </span>
    </span>
  );
}

export function OnboardingGroupBindingSelect(props: {
  upstreamGroupName: string;
  upstreamPlatform: string | null;
  groups: BindingGroupOption[];
  value: string[];
  disabled: boolean;
  disabledReason: string | null;
  onValueChange: (value: string[]) => void;
}) {
  const compatibleGroups = compatibleOnboardingLocalGroups(
    { platform: props.upstreamPlatform },
    props.groups,
  );
  const groupsByID = new Map(
    compatibleGroups.flatMap((group) => (group.id ? [[group.id, group]] : [])),
  );
  const control = (
    <MultiSelect
      options={compatibleGroups.flatMap((group) =>
        group.id ? [{ value: group.id, label: group.name }] : [],
      )}
      selected={props.value}
      onChange={props.onValueChange}
      title="选择本地分组"
      searchPlaceholder="搜索本地分组"
      emptyText="没有匹配的本地分组"
      unknownValueLabel="所选分组不可用"
      ariaLabel={`${props.upstreamGroupName} 本地分组`}
      disabled={props.disabled}
      className="min-h-7 min-w-44 rounded-md py-0.5"
      maxVisibleChips={2}
      renderOption={(option) => {
        const group = groupsByID.get(option.value);
        return group ? (
          <OnboardingGroupBindingOption group={group} />
        ) : (
          <span className="truncate">{option.label}</span>
        );
      }}
    />
  );
  if (!props.disabledReason) return control;
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            className="block w-full"
            aria-label={`${props.upstreamGroupName} 不可添加：${props.disabledReason}`}
          />
        }
      >
        {control}
      </TooltipTrigger>
      <TooltipContent>{props.disabledReason}</TooltipContent>
    </Tooltip>
  );
}
