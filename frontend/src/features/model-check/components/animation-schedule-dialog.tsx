import { zodResolver } from "@hookform/resolvers/zod";
import { FieldError } from "@/components/field-error";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { api, type AnimationSchedule } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
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

const numericFields = [
  { name: "interval_minutes", label: "检测间隔（分钟）", min: 1, max: 1440 },
  { name: "timeout_seconds", label: "请求超时（秒）", min: 5, max: 120 },
] as const;

export function AnimationScheduleDialog(props: {
  accountID: string;
  accountName: string;
  model: string;
  schedule?: AnimationSchedule;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const [confirmation, setConfirmation] = useState<AnimationScheduleForm | null>(null);
  const form = useForm<AnimationScheduleForm>({
    resolver: zodResolver(animationScheduleSchema),
    defaultValues: props.schedule ?? {
      account_id: props.accountID,
      model: props.model,
      enabled: false,
      interval_minutes: 60,
      timeout_seconds: 120,
      version: 0,
    },
  });
  const save = useMutation({
    mutationFn: api.saveAnimationSchedule,
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
    if (value.enabled) setConfirmation(value);
    else save.mutate(value);
  });
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !save.isPending) props.onClose();
        }}
      >
        <DialogContent showCloseButton={!save.isPending}>
          <DialogHeader>
            <DialogTitle>自动检测设置 · {props.accountName}</DialogTitle>
            <DialogDescription>
              由服务器持续执行，关闭页面后仍会检测并产生 API 用量。服务器重启后重新等待设定间隔。
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submit} className="contents">
            <DialogBody className="space-y-4">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={form.watch("enabled")}
                  disabled={save.isPending}
                  onCheckedChange={(value) => form.setValue("enabled", value)}
                />
                开启自动检测
              </label>
              <label className="block space-y-1 text-sm">
                检测模型
                <Input
                  {...form.register("model")}
                  disabled={save.isPending}
                  aria-invalid={!!form.formState.errors.model}
                />
              </label>
              <FieldError message={form.formState.errors.model?.message} />
              {numericFields.map((field) => (
                <div key={field.name}>
                  <label className="block space-y-1 text-sm">
                    {field.label}
                    <Input
                      type="number"
                      min={field.min}
                      max={field.max}
                      {...form.register(field.name, { valueAsNumber: true })}
                      disabled={save.isPending}
                      aria-invalid={!!form.formState.errors[field.name]}
                    />
                  </label>
                  <FieldError message={form.formState.errors[field.name]?.message} />
                </div>
              ))}
              <p className="text-muted-foreground text-xs">
                每次检测结束后重新计时。关闭自动检测会阻止后续请求，已开始的任务可在检测结果中取消。
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
                disabled={save.isPending || (!props.schedule && !form.watch("enabled"))}
              >
                {save.isPending ? "正在保存…" : "保存设置"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmActionDialog
        open={confirmation !== null}
        title="确认开启自动检测"
        description={`将对账号 ${props.accountName}（ID ${props.accountID}）使用模型 ${confirmation?.model ?? ""}，每 ${confirmation?.interval_minutes ?? 60} 分钟生成一次动画并产生 API 用量。`}
        confirmLabel="确认保存并开启"
        pending={save.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation) save.mutate(confirmation);
        }}
      />
    </>
  );
}
