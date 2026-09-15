import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { useState, type ReactElement } from "react";
import { ChevronDown, Trash2 } from "lucide-react";

import type { PricingGroup } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { ExchangeGroupOption } from "./exchange-group-option";

export function ExchangeGroupSetEditor(props: {
  setIndex: number;
  name: string;
  groupSet: string[];
  groups: PricingGroup[];
  exchangeSetByGroup: Map<string, number>;
  exchangeSetNames: string[];
  groupMinimums?: Record<string, string>;
  onMinimumChange: (groupID: string, value: string) => void;
  onToggle: (setIndex: number, groupID: string, checked: boolean) => void;
  onNameChange: (setIndex: number, name: string) => void;
  onRemove: (setIndex: number) => void;
}): ReactElement {
  // 默认只展开第一条规则，避免多组配置同时占满页面；用户可以按需展开其余规则。
  const [expanded, setExpanded] = useState(props.setIndex === 0);
  const setNumber = props.setIndex + 1;
  const contentID = `exchange-set-content-${setNumber}`;
  const orderedGroups = useDictionaryOrder("group", props.groups, (group) => group.id);
  const selectedPlatforms = new Set(
    props.groups
      .filter((group) => props.groupSet.includes(group.id))
      .map((group) => group.platform || "未标注平台"),
  );
  const selectedPlatform = selectedPlatforms.size === 1 ? [...selectedPlatforms][0] : undefined;
  const platformMismatch = selectedPlatforms.size > 1;
  const visibleGroups = selectedPlatform
    ? orderedGroups.filter(
        (group) =>
          props.groupSet.includes(group.id) ||
          (group.platform || "未标注平台") === selectedPlatform,
      )
    : orderedGroups;
  const sections = new Map<string, PricingGroup[]>();
  for (const group of visibleGroups) {
    const platform = group.platform || "未标注平台";
    const section = sections.get(platform) ?? [];
    section.push(group);
    sections.set(platform, section);
  }
  const orderedSections = useDictionaryOrder(
    "platform",
    [...sections.entries()],
    (section) => section[0],
  );
  const complete = props.groupSet.length >= 2 && !platformMismatch;
  let statusLabel = `已选 ${props.groupSet.length} / 至少 2 个`;
  if (platformMismatch) statusLabel = "平台混用";
  else if (complete) statusLabel = `${props.groupSet.length} 个分组`;

  return (
    <section
      className="min-w-0 overflow-hidden rounded-xl border bg-card shadow-sm"
      data-testid={`exchange-set-${setNumber}`}
      aria-labelledby={`exchange-set-title-${setNumber}`}
    >
      <div className={cn("space-y-2.5 bg-muted/30 px-4 py-3", expanded && "border-b")}>
        <div
          className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2"
          data-slot="exchange-set-heading"
        >
          <span
            id={`exchange-set-title-${setNumber}`}
            className="text-muted-foreground shrink-0 text-xs font-medium"
          >
            规则 {setNumber}
          </span>
          <Input
            className="min-w-0 bg-background font-medium"
            value={props.name}
            maxLength={64}
            aria-label={`互换组 ${setNumber} 规则名称`}
            placeholder={`互换组 ${setNumber}`}
            onChange={(event) => props.onNameChange(props.setIndex, event.target.value)}
          />
          <div className="flex shrink-0 items-center gap-1">
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`${expanded ? "收起" : "展开"}互换组 ${setNumber}`}
                    aria-expanded={expanded}
                    aria-controls={contentID}
                    onClick={() => setExpanded((value) => !value)}
                  />
                }
              >
                <ChevronDown
                  className={cn("transition-transform", expanded && "rotate-180")}
                  aria-hidden="true"
                />
              </TooltipTrigger>
              <TooltipContent>{expanded ? "收起互换组" : "展开互换组"}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-destructive"
                    aria-label={`删除互换组 ${setNumber}`}
                    onClick={() => props.onRemove(props.setIndex)}
                  />
                }
              >
                <Trash2 aria-hidden="true" />
              </TooltipTrigger>
              <TooltipContent>删除互换组</TooltipContent>
            </Tooltip>
          </div>
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-2" data-slot="exchange-set-summary">
          <Badge variant={complete ? "outline" : "warning"}>{statusLabel}</Badge>
          {selectedPlatform ? <Badge variant="secondary">{selectedPlatform}</Badge> : null}
          <span className="text-muted-foreground text-xs tabular-nums">
            {visibleGroups.filter((group) => group.available).length} 个可用
          </span>
        </div>
      </div>
      <div
        id={contentID}
        className="space-y-4 bg-muted/10 p-4"
        data-testid={`exchange-set-options-${setNumber}`}
        role="group"
        aria-label={`互换组 ${setNumber} 可选分组`}
        hidden={!expanded}
      >
        {visibleGroups.length === 0 ? (
          <div className="grid min-h-28 place-content-center gap-1 rounded-md border border-dashed px-4 py-5 text-center">
            <p className="text-sm font-medium">暂无可选分组</p>
            <p className="text-muted-foreground text-xs leading-5">
              请先在分组管理中配置可用的业务分组，然后刷新价格数据。
            </p>
          </div>
        ) : null}
        {orderedSections.map(([platform, groups]) => (
          <div key={platform} data-platform-section={platform} className="space-y-2.5">
            {sections.size > 1 || !selectedPlatform ? (
              <div className="flex min-w-0 items-center gap-2 px-0.5">
                <span className="text-foreground/70 min-w-0 text-xs font-semibold [overflow-wrap:anywhere]">
                  {platform}
                </span>
                <span className="bg-border h-px min-w-4 flex-1" aria-hidden="true" />
                <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                  {groups.filter((group) => group.available).length} 个可用
                </span>
              </div>
            ) : null}
            <div className="space-y-2">
              <div
                className="grid auto-rows-fr items-start gap-3 sm:grid-cols-2 xl:grid-cols-3"
                data-slot="exchange-group-grid"
              >
                {groups.map((group) => {
                  const assignedSet = props.exchangeSetByGroup.get(group.id);
                  const selected = assignedSet === props.setIndex;
                  let assignedSetName: string | undefined;
                  if (assignedSet !== undefined && !selected) {
                    assignedSetName =
                      props.exchangeSetNames[assignedSet] || `互换组 ${assignedSet + 1}`;
                  }
                  return (
                    <ExchangeGroupOption
                      key={group.id}
                      group={group}
                      setIndex={props.setIndex}
                      selected={selected}
                      assignedSetName={assignedSetName}
                      wrongPlatform={Boolean(
                        selectedPlatform && (group.platform || "未标注平台") !== selectedPlatform,
                      )}
                      minimum={props.groupMinimums?.[group.id]}
                      onToggle={props.onToggle}
                      onMinimumChange={props.onMinimumChange}
                    />
                  );
                })}
              </div>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
