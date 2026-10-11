import { memo, useMemo, type ReactElement } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { WorkbenchAccount } from "../types";
import { collectAccountGroups } from "../lib/account-groups";
import { VirtualGroupFilter } from "./virtual-group-filter";

export const AccountGroupFilter = memo(function AccountGroupFilter(props: {
  accounts: readonly WorkbenchAccount[];
  value: string;
  onChange: (value: string) => void;
}): ReactElement {
  const groups = useMemo(() => collectAccountGroups(props.accounts), [props.accounts]);
  const options = useMemo(
    () =>
      Array.from(groups.size > 100 ? [] : groups.values(), (group) => (
        <SelectItem key={group.id} value={group.id}>
          {group.name || `分组 #${group.id}`}
        </SelectItem>
      )),
    [groups],
  );
  if (groups.size > 100) {
    return <VirtualGroupFilter groups={groups} value={props.value} onChange={props.onChange} />;
  }
  return (
    <Select
      value={props.value}
      onValueChange={(value, details) => {
        // A missing selected option must not broaden the account scope.
        if (details.reason === "none" && props.value !== "all" && !groups.has(props.value)) {
          details.cancel();
          return;
        }
        if (value !== null) props.onChange(value);
      }}
    >
      <SelectTrigger aria-label="筛选账号分组">
        <SelectValue>
          {props.value === "all"
            ? "全部分组"
            : groups.get(props.value)?.name || `分组 #${props.value}`}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">全部分组</SelectItem>
        {options}
      </SelectContent>
    </Select>
  );
});
