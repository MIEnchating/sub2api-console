import { useMemo, type ReactElement } from "react";
import type { AccountStatus } from "@/api";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import {
  defaultAnimationFilters,
  priorityLabels,
  priorityOptions,
  type AnimationFilters,
} from "../lib/animation-filters";

const priorities = [...priorityOptions];

export function AnimationAccountFilters(props: {
  accounts: AccountStatus[];
  value: AnimationFilters;
  onChange: (value: AnimationFilters) => void;
}): ReactElement {
  const groups = useMemo(
    () => [...new Set(props.accounts.flatMap((account) => account.groups))].sort(),
    [props.accounts],
  );
  const platforms = useMemo(
    () => [
      ...new Set(props.accounts.flatMap((account) => (account.platform ? [account.platform] : []))),
    ],
    [props.accounts],
  );
  const orderedPlatforms = useDictionaryOrder("platform", platforms, (platform) => platform);
  return (
    <div
      role="group"
      aria-label="动画账号筛选"
      className="flex w-full min-w-0 items-center gap-2 overflow-x-auto"
    >
      <Input
        className="w-64 min-w-48 shrink-0"
        value={props.value.query}
        onChange={(event) => props.onChange({ ...props.value, query: event.target.value })}
        aria-label="搜索动画检测账号"
        placeholder="搜索账号、ID、分组或 Host"
      />
      <FilterMenu
        label="分组"
        options={groups}
        value={props.value.group}
        onValueChange={(group) => props.onChange({ ...props.value, group })}
      />
      <FilterMenu
        label="平台"
        options={orderedPlatforms}
        value={props.value.platform}
        onValueChange={(platform) => props.onChange({ ...props.value, platform })}
      />
      <FilterMenu
        label="优先状态"
        options={priorities}
        optionLabel={(value) => priorityLabels[value]}
        value={props.value.priority}
        onValueChange={(priority) => props.onChange({ ...props.value, priority })}
      />
      <Button
        type="button"
        variant="ghost"
        disabled={
          !props.value.query && !props.value.group && !props.value.platform && !props.value.priority
        }
        onClick={() => props.onChange(defaultAnimationFilters)}
      >
        重置筛选
      </Button>
    </div>
  );
}
