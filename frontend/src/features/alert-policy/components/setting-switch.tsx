import { useId, type ReactElement } from "react";
import { FieldLabel } from "@/components/field-help-tooltip";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

export function SettingSwitch(props: {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
}): ReactElement {
  const switchId = useId();
  return (
    <div
      data-slot="setting-switch"
      className="flex min-h-11 min-w-0 items-center justify-between gap-3 rounded-md py-2"
    >
      <FieldLabel
        label={props.label}
        description={props.description}
        htmlFor={switchId}
        className={cn(
          "text-sm font-normal [overflow-wrap:anywhere]",
          props.disabled ? "text-muted-foreground cursor-not-allowed" : "cursor-pointer",
        )}
      />
      <Switch
        id={switchId}
        checked={props.checked}
        disabled={props.disabled}
        onCheckedChange={props.onCheckedChange}
        aria-label={props.label}
      />
    </div>
  );
}
