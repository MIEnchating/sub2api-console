import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function OnboardingGroupBindingSelect(props: {
  upstreamGroupName: string;
  groups: Array<{ id: string | null; name: string }>;
  value: string[];
  disabled: boolean;
  disabledReason: string | null;
  onValueChange: (value: string[]) => void;
}) {
  const control = (
    <Select
      multiple
      value={props.value}
      itemToStringLabel={(value) =>
        props.groups.find((group) => group.id === value)?.name ?? "所选分组不可用"
      }
      disabled={props.disabled}
      onValueChange={props.onValueChange}
    >
      <SelectTrigger
        size="sm"
        className="w-full min-w-44"
        aria-label={`${props.upstreamGroupName} 本地分组`}
      >
        <SelectValue placeholder="选择本地分组" />
      </SelectTrigger>
      <SelectContent className="min-w-56" align="start">
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
