import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FolderPlus } from "lucide-react";
import type { NewAPIChannel, NewAPIChannelGroup } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  channelGroupNameSchema,
  channelGroupsSchema,
  type ChannelGroupsValues,
} from "../lib/channel-group-schema";

import { ChannelGroupCard } from "./channel-group-card";
import { ChannelGroupMemberDialog } from "./channel-group-member-dialog";

export function ChannelGroupDialog(props: {
  platformId: string;
  channels: NewAPIChannel[];
  groups: NewAPIChannelGroup[];
  selected: NewAPIChannel[];
  version: string;
  pending: boolean;
  onClose: () => void;
  onSave: (groups: NewAPIChannelGroup[], version: string) => Promise<void>;
}) {
  const [version] = useState(props.version);
  const [addingGroupID, setAddingGroupID] = useState<string | null>(null);
  const [addedChannels, setAddedChannels] = useState<NewAPIChannel[]>([]);
  const channelNames = useMemo(
    () =>
      new Map(
        [...props.channels, ...props.selected, ...addedChannels].map((channel) => [
          channel.id,
          channel.name,
        ]),
      ),
    [props.channels, props.selected, addedChannels],
  );
  const form = useForm<ChannelGroupsValues>({
    resolver: zodResolver(channelGroupsSchema),
    defaultValues: { groups: props.groups, newName: "" },
  });
  const groups = form.watch("groups");
  const selectedIDs = props.selected.map((item) => item.id);
  const addingGroup = groups.find((group) => group.id === addingGroupID);
  function create(): void {
    const parsed = channelGroupNameSchema.safeParse(form.getValues("newName"));
    if (!parsed.success) {
      form.setError("newName", { message: parsed.error.issues[0].message });
      return;
    }
    if (groups.some((group) => group.name.trim() === parsed.data)) {
      form.setError("newName", { message: "分组名称已存在" });
      return;
    }
    form.setValue("groups", [
      ...groups,
      {
        // getRandomValues also works when the console is accessed over HTTP.
        id: `group-${crypto.getRandomValues(new Uint32Array(4)).join("-")}`,
        name: parsed.data,
        channel_ids: [],
      },
    ]);
    form.setValue("newName", "");
    form.clearErrors("newName");
  }
  function changeMembers(index: number, ids: string[]): void {
    form.setValue(`groups.${index}.channel_ids`, ids, { shouldValidate: true });
  }
  async function submit(values: ChannelGroupsValues): Promise<void> {
    try {
      await props.onSave(values.groups, version);
      props.onClose();
    } catch {
      // Keep the draft open; the mutation reports the error.
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent width="wide" height="large" className="flex flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>渠道分组</DialogTitle>
          <DialogDescription>为渠道分类管理，可直接添加渠道，完成后保存分组。</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col gap-4" onSubmit={form.handleSubmit(submit)}>
          <DialogBody className="flex min-h-0 flex-col gap-4 overflow-y-auto">
            <div className="flex flex-wrap items-start gap-2 rounded-xl border border-dashed bg-muted/20 p-3 sm:p-4">
              <div className="min-w-0 flex-1 basis-48">
                <label htmlFor="channel-group-new" className="mb-1.5 block text-sm font-medium">
                  新建分组名称
                </label>
                <Input
                  id="channel-group-new"
                  {...form.register("newName")}
                  disabled={props.pending}
                  aria-invalid={!!form.formState.errors.newName}
                  aria-describedby="channel-group-new-error"
                  placeholder="例如：生产主渠道"
                />
                <p id="channel-group-new-error" className="text-destructive text-xs">
                  {form.formState.errors.newName?.message}
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                className="mt-6"
                disabled={props.pending || groups.length >= 100}
                onClick={create}
              >
                <FolderPlus aria-hidden="true" />
                新建分组
              </Button>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
              <span>共 {groups.length} 个分组</span>
              {props.selected.length > 0 && (
                <span>列表已选 {props.selected.length} 个渠道，可快捷加入分组</span>
              )}
            </div>
            {groups.length === 0 && (
              <p className="text-muted-foreground py-6 text-center text-sm">暂无渠道分组</p>
            )}
            <div className="grid min-w-0 gap-4">
              {groups.map((group, index) => (
                <ChannelGroupCard
                  key={group.id}
                  group={group}
                  index={index}
                  form={form}
                  selectedIDs={selectedIDs}
                  channelNames={channelNames}
                  pending={props.pending}
                  onAdd={() => setAddingGroupID(group.id)}
                  onChangeMembers={(ids) => changeMembers(index, ids)}
                  onDelete={() =>
                    form.setValue(
                      "groups",
                      groups.filter((item) => item.id !== group.id),
                    )
                  }
                />
              ))}
            </div>
          </DialogBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={props.pending}
              onClick={props.onClose}
            >
              取消
            </Button>
            <Button type="submit" disabled={props.pending}>
              {props.pending ? "正在保存..." : "保存分组"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
      {addingGroup && (
        <ChannelGroupMemberDialog
          platformId={props.platformId}
          groupName={addingGroup.name}
          memberIDs={addingGroup.channel_ids}
          onClose={() => setAddingGroupID(null)}
          onAdd={(channels) => {
            changeMembers(
              groups.findIndex((group) => group.id === addingGroup.id),
              [...new Set([...addingGroup.channel_ids, ...channels.map((channel) => channel.id)])],
            );
            setAddedChannels((current) => [...current, ...channels]);
            setAddingGroupID(null);
          }}
        />
      )}
    </Dialog>
  );
}
