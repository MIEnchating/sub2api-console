import { useMemo, type ReactElement, type ReactNode } from "react";
import { ArrowDownUp, RotateCcw } from "lucide-react";
import type { AccountStatus } from "@/api";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { animationAccountSortOptions, type AnimationAccountSort } from "../constants";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useDictionaryOrder, useDictionaryLabel } from "@/hooks/use-dictionary-order";
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
  resultNavigation?: ReactNode;
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
  const platformLabel = useDictionaryLabel("platform", (value) => value);
  const orderedPlatforms = useDictionaryOrder("platform", platforms, (platform) => platform);
  const orderedGroups = useDictionaryOrder("group", groups, (group) => group, "name");
  return (
    <div
      role="group"
      aria-label="动画账号筛选"
      className="flex w-full min-w-0 flex-wrap items-center gap-2"
    >
      <div className="flex w-full min-w-0 items-center gap-2 sm:w-auto">
        <Input
          className="w-0 min-w-0 flex-1 sm:w-52 sm:flex-none"
          value={props.value.query}
          onChange={(event) => props.onChange({ ...props.value, query: event.target.value })}
          aria-label={props.searchLabel ?? "搜索动画检测账号"}
          placeholder="搜索账号、ID、分组或 Host"
        />
        {props.resultNavigation}
      </div>
      <FilterMenu
        label="分组"
        dictionaryKind="group"
        options={orderedGroups}
        value={props.value.group}
        onValueChange={(group) => props.onChange({ ...props.value, group })}
      />
      <FilterMenu
        label="平台"
        dictionaryKind="platform"
        options={orderedPlatforms}
        optionLabel={platformLabel}
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
      <Select<AnimationAccountSort>
        value={props.value.sort}
        onValueChange={(sort) => {
          if (sort) props.onChange({ ...props.value, sort });
        }}
        items={animationAccountSortOptions}
        itemToStringLabel={(value) =>
          animationAccountSortOptions.find((option) => option.value === value)?.label ?? "默认排序"
        }
      >
        <SelectTrigger aria-label="账号排序" className="w-full sm:w-44">
          <ArrowDownUp aria-hidden="true" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {animationAccountSortOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
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
                !props.value.priority &&
                props.value.sort === "default"
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
