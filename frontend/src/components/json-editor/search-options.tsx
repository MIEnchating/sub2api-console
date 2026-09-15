import { Menu } from "@base-ui/react/menu";
import type { SearchQuery } from "@codemirror/search";
import { CaseSensitive, Check, Regex, SlidersHorizontal, WholeWord } from "lucide-react";
import type { ReactElement } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const searchOptions = [
  { key: "caseSensitive", label: "区分大小写", icon: CaseSensitive },
  { key: "regexp", label: "正则表达式", icon: Regex },
  { key: "wholeWord", label: "全字匹配", icon: WholeWord },
] as const;

export function SearchOptions(props: {
  query: SearchQuery;
  onChange: (changes: Partial<SearchQuery>) => void;
}): ReactElement {
  const active = searchOptions.some((option) => props.query[option.key]);
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger
          render={
            <DropdownMenuTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="匹配选项"
                  className={cn(
                    "relative text-muted-foreground",
                    active && "bg-accent text-accent-foreground",
                  )}
                />
              }
            />
          }
        >
          <SlidersHorizontal aria-hidden="true" />
          {active && (
            <span
              className="absolute right-1.5 top-1.5 size-1 rounded-full bg-primary"
              aria-hidden="true"
            />
          )}
        </TooltipTrigger>
        <TooltipContent>匹配选项</TooltipContent>
      </Tooltip>
      <DropdownMenuContent aria-label="匹配选项">
        {searchOptions.map((option) => (
          <Menu.CheckboxItem
            key={option.key}
            checked={props.query[option.key]}
            onCheckedChange={(checked) => props.onChange({ [option.key]: checked })}
            closeOnClick={false}
            className="flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground"
          >
            <option.icon className="size-4 text-muted-foreground" aria-hidden="true" />
            <span className="flex-1">{option.label}</span>
            <span className="size-4">
              <Menu.CheckboxItemIndicator>
                <Check className="size-4" aria-hidden="true" />
              </Menu.CheckboxItemIndicator>
            </span>
          </Menu.CheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
