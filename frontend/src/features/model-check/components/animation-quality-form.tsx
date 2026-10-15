import { useId, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type AnimationResult, type AnimationQualityReview } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { notifyOperationError } from "@/lib/operation-feedback";
import { animationQualityLabels } from "../constants";
import {
  animationQualityFormSchema,
  type AnimationQualityForm,
} from "../lib/animation-quality-schema";

export function AnimationQualityForm(props: {
  review: AnimationQualityReview;
  result: AnimationResult;
  taskID: string;
}): ReactElement {
  const id = useId();
  const client = useQueryClient();
  const form = useForm<AnimationQualityForm>({
    resolver: zodResolver(animationQualityFormSchema),
    defaultValues: {
      label: props.review.annotation?.label ?? "unknown",
      reason: props.review.annotation?.reason ?? "",
      source_group: props.review.annotation?.source_group ?? "",
    },
  });
  const save = useMutation({
    mutationFn: (values: AnimationQualityForm) =>
      api.saveAnimationQuality(props.taskID, {
        ...values,
        account_id: props.result.account_id,
        request_id: props.result.request_id,
        version: props.review.annotation?.version ?? 0,
      }),
    onSuccess: (review) => {
      void client.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
      client.setQueryData(
        ["animation-quality", props.taskID, props.result.account_id, props.result.request_id],
        review,
      );
      toast.success("质量标注已保存");
    },
    onError: (error) => notifyOperationError(error, "质量标注保存失败"),
  });
  return (
    <form onSubmit={form.handleSubmit((values) => save.mutate(values))} className="space-y-3">
      <fieldset disabled={save.isPending} className="space-y-3">
        <legend className="mb-2 font-medium">人工质量标签</legend>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
          {(["normal", "suspicious", "unknown"] as const).map((label) => (
            <label
              key={label}
              className="flex cursor-pointer items-center gap-2 rounded-lg border p-3 has-checked:border-primary has-checked:bg-primary/5"
            >
              <input type="radio" value={label} {...form.register("label")} />
              {animationQualityLabels[label]}
            </label>
          ))}
        </div>
        <div className="space-y-1.5">
          <label htmlFor={`${id}-reason`}>判断理由（必填）</label>
          <Textarea
            id={`${id}-reason`}
            {...form.register("reason")}
            aria-invalid={Boolean(form.formState.errors.reason)}
            aria-describedby={form.formState.errors.reason ? `${id}-reason-error` : undefined}
            placeholder="例如：车轮旋转正常，但脚踏与腿部脱节"
          />
          {form.formState.errors.reason ? (
            <p id={`${id}-reason-error`} className="text-xs text-destructive">
              {form.formState.errors.reason.message}
            </p>
          ) : null}
        </div>
        <div className="space-y-1.5">
          <label htmlFor={`${id}-group`}>样本分组（可选）</label>
          <Input
            id={`${id}-group`}
            {...form.register("source_group")}
            aria-invalid={Boolean(form.formState.errors.source_group)}
            aria-describedby={`${id}-group-help`}
          />
          <p id={`${id}-group-help`} className="text-xs text-muted-foreground">
            相同或近似模板请填同一分组，避免训练与测试重复；留空按源码指纹分组。
          </p>
          {form.formState.errors.source_group ? (
            <p className="text-xs text-destructive">{form.formState.errors.source_group.message}</p>
          ) : null}
        </div>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? "正在保存" : "保存质量标注"}
        </Button>
      </fieldset>
      {props.review.annotation ? (
        <p className="text-xs text-muted-foreground">
          已标注：{animationQualityLabels[props.review.annotation.label]} ·{" "}
          {new Date(props.review.annotation.updated_at).toLocaleString("zh-CN")}
        </p>
      ) : null}
    </form>
  );
}
