import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { api, type AnimationSchedule } from "@/api";
import { FieldError } from "@/components/field-error";
import { AnimationScheduleConfirmation } from "./animation-schedule-confirmation";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { notifyOperationError } from "@/lib/operation-feedback";
import { animationScheduleSchema, type AnimationScheduleForm } from "../lib/animation-schema";
import { allPrecheckQuestions, precheckQuestionSummary } from "../constants";
import { PrecheckQuestionSelector } from "./precheck-question-selector";
import { AnimationScheduleTiming } from "./animation-schedule-timing";

export type ScheduleTarget = {
  accountID: string;
  accountName: string;
  schedule?: AnimationSchedule;
};

export function AnimationScheduleDialog(props: {
  accountID: string;
  accountName: string;
  model: string;
  mode?: "animation" | "precheck";
  schedule?: AnimationSchedule;
  targets?: ScheduleTarget[];
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const mode = props.mode ?? "animation";
  const label = mode === "precheck" ? "前置检测" : "动画检测";
  const [targets] = useState<ScheduleTarget[]>(
    () =>
      props.targets ?? [
        { accountID: props.accountID, accountName: props.accountName, schedule: props.schedule },
      ],
  );
  const [confirmation, setConfirmation] = useState<AnimationSchedule[] | null>(null);
  const form = useForm<AnimationScheduleForm>({
    resolver: zodResolver(animationScheduleSchema),
    defaultValues: {
      account_id: props.accountID,
      model: props.model,
      enabled: false,
      interval_minutes: 60,
      timeout_seconds: 120,
      version: 0,
      ...props.schedule,
      daily_time: undefined,
      daily_times: props.schedule?.daily_times ?? [props.schedule?.daily_time ?? ""],
      mode,
      timezone: props.schedule?.schedule_type === "daily" ? "Asia/Shanghai" : undefined,
    },
  });
  const save = useMutation({
    mutationFn: (values: AnimationSchedule[]) =>
      props.targets ? api.saveAnimationSchedules(values) : api.saveAnimationSchedule(values[0]),
    onSuccess: (values) => {
      client.setQueryData(["model-animation", "schedules"], values);
      props.onClose();
    },
    onError: (error) => {
      setConfirmation(null);
      notifyOperationError(error, "自动检测设置保存失败");
      void client.invalidateQueries({ queryKey: ["model-animation", "schedules"] });
    },
  });
  const submit = form.handleSubmit((value) => {
    const payload = targets
      .filter((target) => value.enabled || target.schedule)
      .map((target): AnimationSchedule => ({
        ...value,
        account_id: target.accountID,
        version: target.schedule?.version ?? 0,
        mode: mode === "precheck" ? "precheck" : undefined,
        precheck_questions:
          mode === "precheck" ? (value.precheck_questions ?? allPrecheckQuestions) : undefined,
        daily_time: undefined,
        daily_times:
          value.schedule_type === "daily" ? value.daily_times?.slice().sort() : undefined,
        timezone: value.schedule_type === "daily" ? "Asia/Shanghai" : undefined,
      }));
    if (value.enabled || props.targets) setConfirmation(payload);
    else save.mutate(payload);
  });
  const enabled = form.watch("enabled");
  const timing =
    form.watch("schedule_type") === "daily"
      ? `每天 ${form.watch("daily_times")?.slice().sort().join("、") ?? ""}（北京时间）`
      : `每 ${form.watch("interval_minutes")} 分钟`;
  const action =
    mode === "precheck"
      ? precheckQuestionSummary(form.getValues("precheck_questions"))
      : "动画检测";
  const confirmationAction = enabled
    ? `使用模型 ${form.getValues("model")}，${timing}执行${action}并产生 API 用量`
    : "关闭所选账号的此类自动检测，未配置的账号自动跳过";
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
            <DialogTitle>
              {props.targets ? "批量" : ""}自动{label}设置 · {props.accountName}
            </DialogTitle>
            <DialogDescription>
              仅设置{label}。关闭页面后服务器仍会按计划执行并产生 API 用量。
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={submit}
            noValidate
            className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-4"
          >
            <DialogBody className="space-y-4">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={enabled}
                  disabled={save.isPending}
                  onCheckedChange={(value) => form.setValue("enabled", value)}
                />
                开启自动检测
              </label>
              {mode === "precheck" && (
                <div className="space-y-1">
                  <PrecheckQuestionSelector
                    value={form.watch("precheck_questions") ?? allPrecheckQuestions}
                    onChange={(value) =>
                      form.setValue("precheck_questions", value, {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                    }
                    disabled={save.isPending}
                  />
                  <FieldError message={form.formState.errors.precheck_questions?.message} />
                </div>
              )}
              <label className="block space-y-1 text-sm">
                检测模型
                <Input
                  {...form.register("model")}
                  disabled={save.isPending}
                  aria-invalid={!!form.formState.errors.model}
                />
              </label>
              <FieldError message={form.formState.errors.model?.message} />
              <AnimationScheduleTiming form={form} pending={save.isPending} />
              <label className="block space-y-1 text-sm">
                请求超时（秒）
                <Input
                  type="number"
                  min={5}
                  max={120}
                  {...form.register("timeout_seconds", { valueAsNumber: true })}
                  disabled={save.isPending}
                  aria-invalid={!!form.formState.errors.timeout_seconds}
                />
              </label>
              <FieldError message={form.formState.errors.timeout_seconds?.message} />
              <p className="text-muted-foreground text-xs">
                关闭自动检测会阻止后续请求，已开始的任务可在检测结果中取消。另一类检测的设置独立保留。
              </p>
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
              <Button
                type="submit"
                disabled={
                  save.isPending || (!enabled && !targets.some((target) => target.schedule))
                }
              >
                {save.isPending ? "正在保存…" : "保存设置"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <AnimationScheduleConfirmation
        open={confirmation !== null}
        enabled={enabled}
        description={confirmationAction}
        targets={targets}
        pending={save.isPending}
        onClose={() => setConfirmation(null)}
        onConfirm={() => {
          if (confirmation) save.mutate(confirmation);
        }}
      />
    </>
  );
}
