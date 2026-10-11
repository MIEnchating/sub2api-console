import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useRef } from "react";
import { useForm } from "react-hook-form";

import type { AccountModelAutomationSchedule, AccountModelAutomationTaskGroup } from "@/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  automationTaskGroupSchema,
  type AutomationTaskGroupForm,
} from "../lib/automation-task-group";

export function AccountModelAutomationTaskGroupDialog(props: {
  open: boolean;
  group?: AccountModelAutomationTaskGroup;
  schedules: AccountModelAutomationSchedule[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (input: AutomationTaskGroupForm) => void;
}) {
  const form = useForm<AutomationTaskGroupForm>({
    resolver: zodResolver(automationTaskGroupSchema),
    defaultValues: { name: "", schedule_ids: [] },
  });
  const selectedIDs = form.watch("schedule_ids");
  const initializedGroup = useRef<string | null>(null);
  useEffect(() => {
    if (!props.open) {
      initializedGroup.current = null;
      return;
    }
    const available = new Set(props.schedules.map((schedule) => schedule.id));
    const groupID = props.group?.id ?? "";
    if (initializedGroup.current === groupID) {
      const selected = form.getValues("schedule_ids");
      const remaining = selected.filter((id) => available.has(id));
      if (remaining.length !== selected.length) {
        form.setValue("schedule_ids", remaining, { shouldValidate: true, shouldDirty: true });
      }
      return;
    }
    initializedGroup.current = groupID;
    form.reset({
      name: props.group?.name ?? "",
      schedule_ids: (props.group?.schedule_ids ?? []).filter((id) => available.has(id)),
    });
  }, [form, props.group, props.open, props.schedules]);
  const selectedCount = selectedIDs.length;
  const toggle = (id: string): void => {
    const next = selectedIDs.includes(id)
      ? selectedIDs.filter((selectedID) => selectedID !== id)
      : [...selectedIDs, id];
    const selected = new Set(next);
    form.setValue(
      "schedule_ids",
      props.schedules
        .filter((schedule) => selected.has(schedule.id))
        .map((schedule) => schedule.id),
      { shouldValidate: true },
    );
  };
  return (
    <Dialog open={props.open} onOpenChange={(open) => !props.pending && props.onOpenChange(open)}>
      <DialogContent width="medium" height="large">
        <DialogHeader>
          <DialogTitle>{props.group ? "编辑运营任务组" : "添加运营任务组"}</DialogTitle>
          <DialogDescription>选中的运营任务会按列表顺序依次执行。</DialogDescription>
        </DialogHeader>
        <form onSubmit={form.handleSubmit(props.onSave)} className="contents">
          <DialogBody className="grid gap-4">
            <label className="grid gap-1.5 text-sm font-medium">
              任务组名称
              <Input
                aria-label="运营任务组名称"
                placeholder="例如：生产模型同步"
                disabled={props.pending}
                aria-invalid={Boolean(form.formState.errors.name)}
                {...form.register("name")}
              />
              {form.formState.errors.name && (
                <span className="text-destructive text-xs">
                  {form.formState.errors.name.message}
                </span>
              )}
            </label>
            <section aria-label="选择运营任务" className="grid gap-2">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-medium">选择运营任务</h3>
                <span className="text-muted-foreground text-xs">已选择 {selectedCount} 个</span>
              </div>
              <div className="grid max-h-72 gap-1 overflow-y-auto rounded-lg border bg-muted/20 p-2">
                {props.schedules.map((schedule) => (
                  <label
                    key={schedule.id}
                    className="flex min-h-10 items-center gap-2 rounded-md px-2 text-sm hover:bg-background"
                  >
                    <Checkbox
                      checked={selectedIDs.includes(schedule.id)}
                      disabled={props.pending}
                      onCheckedChange={() => toggle(schedule.id)}
                    />
                    <span className="min-w-0 flex-1 truncate">{schedule.name}</span>
                    <span className="text-muted-foreground text-xs">
                      {schedule.enabled ? "定时已启用" : "手动运行"}
                    </span>
                  </label>
                ))}
                {props.schedules.length === 0 && (
                  <p className="text-muted-foreground p-3 text-sm">请先创建至少两个运营任务。</p>
                )}
              </div>
              {form.formState.errors.schedule_ids && (
                <p className="text-destructive text-xs">
                  {form.formState.errors.schedule_ids.message}
                </p>
              )}
            </section>
          </DialogBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={props.pending}
              onClick={() => props.onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={props.pending}>
              {props.pending ? "保存中" : "保存任务组"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
