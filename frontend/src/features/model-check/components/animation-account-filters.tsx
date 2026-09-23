import { useMemo, type ReactElement } from "react";
import { RotateCcw } from "lucide-react";
import type { AccountStatus } from "@/api";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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
  searchLabel?: string;
}): ReactElement {
  const groups = useMemo(
    () => [...new Set(props.accounts.flatMap((account) => account.groups))],
    [props.accounts],
  );
  const platforms = useMemo(
    () => [
      ...new Set(props.accounts.flatMap((account) => (account.platform ? [account.platform] : []))),
    ],
    [props.accounts],
  );
  const orderedPlatforms = useDictionaryOrder("platform", platforms, (platform) => platform);
  const orderedGroups = useDictionaryOrder("group", groups, (group) => group, "name");
  return (
    <div
      role="group"
      aria-label="动画账号筛选"
      className="flex w-full min-w-0 flex-wrap items-center gap-2"
    >
      <Input
        className="w-full min-w-0 sm:w-52"
        value={props.value.query}
        onChange={(event) => props.onChange({ ...props.value, query: event.target.value })}
        aria-label={props.searchLabel ?? "搜索动画检测账号"}
        placeholder="搜索账号、ID、分组或 Host"
      />
      <FilterMenu
        label="分组"
        options={orderedGroups}
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
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="重置筛选"
              disabled={
                !props.value.query &&
                !props.value.group &&
                !props.value.platform &&
                !props.value.priority
              }
              onClick={() => props.onChange(defaultAnimationFilters)}
            />
          }
        >
          <RotateCcw aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>重置筛选</TooltipContent>
      </Tooltip>
    </div>
  );
}
