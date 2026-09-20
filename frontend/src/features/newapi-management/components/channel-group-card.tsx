import type { UseFormReturn } from "react-hook-form";
import { Plus, Trash2, X } from "lucide-react";
import type { NewAPIChannelGroup } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { ChannelGroupsValues } from "../lib/channel-group-schema";

export function ChannelGroupCard(props: {
  group: NewAPIChannelGroup;
  index: number;
  form: UseFormReturn<ChannelGroupsValues>;
  selectedIDs: string[];
  channelNames: Map<string, string>;
  pending: boolean;
  onAdd: () => void;
  onChangeMembers: (ids: string[]) => void;
  onDelete: () => void;
}) {
  const nextIDs = [...new Set([...props.group.channel_ids, ...props.selectedIDs])];
  const errors = props.form.formState.errors.groups?.[props.index];
  return (
    <fieldset
      className="min-w-0 rounded-xl border bg-background/50"
      aria-label={`分组 ${props.index + 1}`}
    >
      <div className="flex min-w-0 flex-wrap items-start gap-3 p-3 sm:p-4">
        <div className="min-w-0 flex-1 basis-48">
          <label
            htmlFor={`channel-group-name-${props.group.id}`}
            className="text-muted-foreground mb-1.5 block text-xs"
          >
            分组名称
          </label>
          <Input
            id={`channel-group-name-${props.group.id}`}
            aria-label={`分组名称 ${props.index + 1}`}
            {...props.form.register(`groups.${props.index}.name`)}
            disabled={props.pending}
            aria-invalid={!!errors?.name}
            aria-describedby={`channel-group-error-${props.group.id}`}
          />
          <p id={`channel-group-error-${props.group.id}`} className="text-destructive text-xs">
            {errors?.name?.message}
          </p>
        </div>
        <div className="flex max-w-full flex-wrap items-center gap-2 sm:pt-5">
          <Button
            type="button"
            variant="outline"
            disabled={props.pending || props.group.channel_ids.length >= 1000}
            onClick={props.onAdd}
          >
            <Plus aria-hidden="true" />
            添加渠道
          </Button>
          {props.selectedIDs.length > 0 && (
            <>
              <Button
                type="button"
                variant="outline"
                disabled={
                  props.pending ||
                  nextIDs.length === props.group.channel_ids.length ||
                  nextIDs.length > 1000
                }
                onClick={() => props.onChangeMembers(nextIDs)}
              >
                加入已选
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={
                  props.pending ||
                  !props.group.channel_ids.some((id) => props.selectedIDs.includes(id))
                }
                onClick={() =>
                  props.onChangeMembers(
                    props.group.channel_ids.filter((id) => !props.selectedIDs.includes(id)),
                  )
                }
              >
                移除已选
              </Button>
            </>
          )}
          <TableActionButton
            label={`删除分组 ${props.group.name}`}
            tone="danger"
            disabled={props.pending}
            onClick={props.onDelete}
          >
            <Trash2 aria-hidden="true" />
          </TableActionButton>
        </div>
      </div>
      <div className="border-t bg-muted/20 px-3 py-3 sm:px-4">
        {props.group.channel_ids.length === 0 ? (
          <p className="text-muted-foreground text-xs">暂无渠道，点击“添加渠道”选择。</p>
        ) : (
          <details className="min-w-0 text-sm">
            <summary className="text-muted-foreground cursor-pointer select-none">
              {props.group.channel_ids.length} 个渠道
            </summary>
            <div
              role="list"
              aria-label={`${props.group.name}的渠道`}
              className="mt-3 flex max-h-48 flex-wrap gap-2 overflow-y-auto"
            >
              {props.group.channel_ids.map((id) => (
                <span
                  role="listitem"
                  key={id}
                  className="bg-background flex min-w-0 max-w-full items-center gap-1 rounded-md border py-0.5 pr-0.5 pl-2.5"
                >
                  <Tooltip>
                    <TooltipTrigger
                      render={<span tabIndex={0} className="min-w-0 truncate text-xs" />}
                    >
                      {props.channelNames.get(id) ?? `渠道 #${id}`}
                    </TooltipTrigger>
                    <TooltipContent role="tooltip">
                      {props.channelNames.get(id) ?? `渠道 #${id}`}
                    </TooltipContent>
                  </Tooltip>
                  {props.channelNames.has(id) && (
                    <span className="text-muted-foreground shrink-0 text-xs">#{id}</span>
                  )}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-destructive"
                    aria-label={`从 ${props.group.name} 移除渠道 ${id}`}
                    disabled={props.pending}
                    onClick={() =>
                      props.onChangeMembers(props.group.channel_ids.filter((value) => value !== id))
                    }
                  >
                    <X aria-hidden="true" />
                  </Button>
                </span>
              ))}
            </div>
          </details>
        )}
        {errors?.channel_ids?.message && (
          <p className="text-destructive mt-2 text-xs">{errors.channel_ids.message}</p>
        )}
      </div>
    </fieldset>
  );
}
