import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

const noBindingValue = "__no_binding__";

export function OnboardingGroupBindingSelect(props: {
  upstreamGroupName: string;
  groups: Array<{ id: string | null; name: string }>;
  value: string | null;
  disabled: boolean;
  disabledReason: string | null;
  onValueChange: (value: string | null) => void;
}) {
  const control = (
    <Select
      value={props.value ?? noBindingValue}
      itemToStringLabel={(value) => {
        if (value === noBindingValue) return "选择本地分组";
        return props.groups.find((group) => group.id === value)?.name ?? "所选分组不可用";
      }}
      disabled={props.disabled}
      onValueChange={(value) => {
        props.onValueChange(value === noBindingValue ? null : value);
      }}
    >
      <SelectTrigger
        size="sm"
        className="w-full min-w-44"
        aria-label={`${props.upstreamGroupName} 绑定到本地分组`}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent className="min-w-56" align="start">
        <SelectItem value={noBindingValue}>暂不添加</SelectItem>
        {props.groups.map((group) => (
          <SelectItem value={group.id!} key={group.id}>
            {group.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
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
