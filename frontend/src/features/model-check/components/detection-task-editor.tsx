import { useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type DetectionTask } from "@/api";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { detectionTaskSchema, type DetectionTaskForm } from "../lib/detection-task-schema";
import { detectionTaskDescription } from "../lib/detection-task-schema";
import { DetectionTaskFields } from "./detection-task-fields";

export function DetectionTaskEditor(props: {
  value?: DetectionTask;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const [confirmation, setConfirmation] = useState<DetectionTaskForm | null>(null);
  const form = useForm<DetectionTaskForm>({
    resolver: zodResolver(detectionTaskSchema),
    defaultValues: {
      id: "",
      version: 0,
      name: "",
      group_ids: [],
      model: "",
      precheck: false,
      terminal: false,
      terminal_rounds: 3,
      automatic: false,
      schedule_type: "interval",
      interval_minutes: 60,
      timeout_seconds: 120,
      ...props.value,
      daily_times: props.value?.daily_times ?? ["09:00"],
      precheck_questions: props.value?.precheck_questions ?? ["candy"],
    },
  });
  const save = useMutation({
    mutationFn: (value: DetectionTaskForm) =>
      api.saveDetectionTask({
        ...value,
        daily_times: value.schedule_type === "daily" ? value.daily_times.slice().sort() : undefined,
        timezone: value.schedule_type === "daily" ? "Asia/Shanghai" : undefined,
      }),
    onSuccess: (values) => {
      client.setQueryData(["model-detection-tasks"], values);
      props.onClose();
    },
    onError: (error) => {
      setConfirmation(null);
      notifyOperationError(error, "检测任务保存失败");
    },
  });
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !save.isPending) props.onClose();
        }}
      >
        <DialogContent
          showCloseButton={!save.isPending}
          className="grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{props.value ? "编辑检测任务" : "新增检测任务"}</DialogTitle>
            <DialogDescription>保存分组和检测流程，手动执行或按计划自动执行。</DialogDescription>
          </DialogHeader>
          <form
            className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-4"
            noValidate
            onSubmit={form.handleSubmit((value) => {
              if (value.automatic) setConfirmation(value);
              else save.mutate(value);
            })}
          >
            <DialogBody>
              {groups.isPending ? (
                <ContentLoading label="正在读取分组" />
              ) : (
                <DetectionTaskFields
                  form={form}
                  groups={groups.data ?? []}
                  pending={save.isPending}
                />
              )}
              {groups.isError && (
                <ContentRetry onRetry={() => void groups.refetch()} pending={groups.isFetching} />
              )}
            </DialogBody>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={save.isPending}
                onClick={props.onClose}
              >
                取消
              </Button>
              <Button type="submit" disabled={save.isPending || !groups.isSuccess}>
                {save.isPending ? "正在保存…" : "保存任务"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmActionDialog
        open={confirmation !== null}
        title="确认自动检测任务"
        description={
          confirmation
            ? `任务“${confirmation.name}”将对分组 ${confirmation.group_ids.map((id) => groups.data?.find((group) => group.id === id)?.name ?? id).join("、")} 的最新账号，使用 ${confirmation.model} ${detectionTaskDescription(confirmation)}，并产生 API 用量。`
            : ""
        }
        confirmLabel="确认保存并开启"
        pending={save.isPending}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation) save.mutate(confirmation);
        }}
      />
    </>
  );
}
