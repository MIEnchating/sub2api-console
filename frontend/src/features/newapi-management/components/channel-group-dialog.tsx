import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FolderPlus, Trash2, X } from "lucide-react";
import type { NewAPIChannel, NewAPIChannelGroup } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { TableActionButton } from "@/components/data-table/table-action-button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  channelGroupNameSchema,
  channelGroupsSchema,
  type ChannelGroupsValues,
} from "../lib/channel-group-schema";

export function ChannelGroupDialog(props: {
  groups: NewAPIChannelGroup[];
  selected: NewAPIChannel[];
  version: string;
  pending: boolean;
  onClose: () => void;
  onSave: (groups: NewAPIChannelGroup[], version: string) => Promise<void>;
}) {
  const [version] = useState(props.version);
  const form = useForm<ChannelGroupsValues>({
    resolver: zodResolver(channelGroupsSchema),
    defaultValues: { groups: props.groups, newName: "" },
  });
  const groups = form.watch("groups");
  const selectedIDs = props.selected.map((item) => item.id);
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
      { id: `group-${crypto.randomUUID()}`, name: parsed.data, channel_ids: [] },
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
      <DialogContent width="wide" height="large">
        <DialogHeader>
          <DialogTitle>渠道分组</DialogTitle>
        </DialogHeader>
        <form
          className="flex min-h-0 flex-1 flex-col overflow-hidden"
          onSubmit={form.handleSubmit(submit)}
        >
          <DialogBody className="flex min-h-0 flex-col gap-4 overflow-y-auto">
            <div className="flex flex-wrap items-start gap-2">
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
            <p className="text-muted-foreground text-sm">已选 {props.selected.length} 个渠道</p>
            {groups.length === 0 && (
              <p className="text-muted-foreground py-6 text-center text-sm">暂无渠道分组</p>
            )}
            <div className="grid min-w-0 gap-4">
              {groups.map((group, index) => (
                <fieldset
                  key={group.id}
                  className="min-w-0 space-y-2 border-b pb-4"
                  aria-label={`分组 ${index + 1}`}
                >
                  <div className="flex flex-wrap items-start gap-2">
                    <div className="min-w-0 flex-1 basis-40">
                      <Input
                        aria-label={`分组名称 ${index + 1}`}
                        {...form.register(`groups.${index}.name`)}
                        disabled={props.pending}
                        aria-invalid={!!form.formState.errors.groups?.[index]?.name}
                        aria-describedby={`channel-group-error-${group.id}`}
                      />
                      <p
                        id={`channel-group-error-${group.id}`}
                        className="text-destructive text-xs"
                      >
                        {form.formState.errors.groups?.[index]?.name?.message}
                      </p>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={
                        props.pending ||
                        selectedIDs.length === 0 ||
                        selectedIDs.every((id) => group.channel_ids.includes(id))
                      }
                      onClick={() =>
                        changeMembers(index, [...new Set([...group.channel_ids, ...selectedIDs])])
                      }
                    >
                      加入已选
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={
                        props.pending || !group.channel_ids.some((id) => selectedIDs.includes(id))
                      }
                      onClick={() =>
                        changeMembers(
                          index,
                          group.channel_ids.filter((id) => !selectedIDs.includes(id)),
                        )
                      }
                    >
                      移除已选
                    </Button>
                    <TableActionButton
                      label={`删除分组 ${group.name}`}
                      tone="danger"
                      disabled={props.pending}
                      onClick={() =>
                        form.setValue(
                          "groups",
                          groups.filter((item) => item.id !== group.id),
                        )
                      }
                    >
                      <Trash2 aria-hidden="true" />
                    </TableActionButton>
                  </div>
                  <details className="min-w-0 text-sm">
                    <summary className="text-muted-foreground cursor-pointer">
                      {group.channel_ids.length} 个渠道
                    </summary>
                    <div className="mt-2 flex max-h-48 flex-wrap gap-2 overflow-y-auto">
                      {group.channel_ids.map((id) => (
                        <span
                          key={id}
                          className="flex min-w-0 max-w-full items-center gap-1 rounded border pl-2"
                        >
                          <span className="truncate">
                            {props.selected.find((channel) => channel.id === id)?.name ??
                              `渠道 #${id}`}
                          </span>
                          <TableActionButton
                            label={`从 ${group.name} 移除渠道 ${id}`}
                            disabled={props.pending}
                            onClick={() =>
                              changeMembers(
                                index,
                                group.channel_ids.filter((value) => value !== id),
                              )
                            }
                          >
                            <X aria-hidden="true" />
                          </TableActionButton>
                        </span>
                      ))}
                    </div>
                  </details>
                  <p className="text-destructive text-xs">
                    {form.formState.errors.groups?.[index]?.channel_ids?.message}
                  </p>
                </fieldset>
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
    </Dialog>
  );
}
