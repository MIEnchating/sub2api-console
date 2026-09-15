import type { ReactElement } from "react";
import type { GroupStatus } from "@/api";
import { Checkbox } from "@/components/ui/checkbox";

export function WorkbenchGroupPicker(props: {
  groups: GroupStatus[];
  value: string[];
  onChange: (value: string[]) => void;
  disabled?: boolean;
}): ReactElement {
  const groups = props.groups.filter((group) => group.id !== null);
  return (
    <fieldset disabled={props.disabled} className="min-w-0 space-y-2">
      <legend className="mb-2 text-sm font-medium">分组范围</legend>
      <div className="grid max-h-44 gap-2 overflow-y-auto rounded-md border p-3 sm:grid-cols-2">
        {groups.map((group) => (
          <label key={group.id} className="flex min-w-0 items-center gap-2 text-sm">
            <Checkbox
              checked={props.value.includes(group.id!)}
              disabled={props.disabled}
              onCheckedChange={(checked) =>
                props.onChange(
                  checked
                    ? [...props.value, group.id!]
                    : props.value.filter((id) => id !== group.id),
                )
              }
            />
            <span className="min-w-0 wrap-anywhere">
              {group.name}（ID {group.id}）
            </span>
          </label>
        ))}
        {!groups.length && <p className="text-sm text-muted-foreground">暂无可选分组</p>}
      </div>
    </fieldset>
  );
}
