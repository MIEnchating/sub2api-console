import { useId, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type AnimationResult, type AnimationQualityReview } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { notifyOperationError } from "@/lib/operation-feedback";
import { animationQualityLabels, animationQualityReasons } from "../constants";
import {
  animationQualityFormSchema,
  type AnimationQualityForm,
} from "../lib/animation-quality-schema";
import { AnimationQualityBadge } from "./animation-quality-badge";
import { AnimationQualityExport } from "./animation-quality-export";
import { AnimationQualitySettingsDialog } from "./animation-quality-settings";

export function AnimationQualityReviewPanel(props: { result: AnimationResult }): ReactElement {
  const result = props.result;
  if (
    result.source_task_status === "running" ||
    result.source_task_status === "queued" ||
    result.source_task_status === "waiting_input"
  ) {
    return <p className="text-sm text-muted-foreground">本批检测完成后可进行质量标注。</p>;
  }
  if (
    !result.source_task_id ||
    !result.request_id ||
    !result.source ||
    !result.prompt ||
    !result.html ||
    result.source_truncated ||
    result.status !== "succeeded" ||
    result.mode === "precheck" ||
    result.precheck
  ) {
    return (
      <p className="text-sm text-muted-foreground">
        该记录缺少完整 HTML、提示词或请求信息，无法作为质量标注样本。请重新检测后标注。
      </p>
    );
  }
  return <QualityReview result={result} taskID={result.source_task_id} />;
}
function QualityReview(props: { result: AnimationResult; taskID: string }): ReactElement {
  const [correcting, setCorrecting] = useState(false);
  const [samplesOpen, setSamplesOpen] = useState(false);
  const disclosureID = useId();
  const key = ["animation-quality", props.taskID, props.result.account_id, props.result.request_id];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      api.animationQuality(props.taskID, props.result.account_id, props.result.request_id, signal),
    staleTime: 30_000,
  });
  if (!query.data) {
    if (query.isError)
      return <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />;
    return <ContentLoading label="正在读取质量标注" />;
  }
  const prediction = query.data.prediction;
  return (
    <section aria-label="作品质量标注" className="mx-auto w-full max-w-4xl space-y-4 text-sm">
      <div className="space-y-3 rounded-xl border bg-background p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="space-y-2">
            <h3 className="font-semibold">自动质量标注</h3>
            <AnimationQualityBadge prediction={prediction} />
          </div>
          <AnimationQualitySettingsDialog />
        </div>
        {prediction.explanation ? (
          <p className="whitespace-pre-wrap rounded-lg bg-muted/40 p-3 leading-7 wrap-anywhere">
            {prediction.explanation}
          </p>
        ) : null}
        {prediction.source === "model_review" ? (
          <p className="text-xs text-muted-foreground wrap-anywhere">
            模型自动评审 · {prediction.model_version}
          </p>
        ) : null}
        {prediction.reason ? (
          <p className="text-muted-foreground">
            {animationQualityReasons[prediction.reason] ?? prediction.reason}
          </p>
        ) : null}
        {prediction.score !== null ? (
          <p>
            模型得分：{prediction.score.toFixed(4)} · {prediction.model_version}
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          仅根据题目和源码评审，未观看实际动画；标签不能证明模型身份。自动结果不会作为人工确认样本。
        </p>
      </div>
      <div className="rounded-xl border bg-background p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="space-y-1">
            <h3 className="font-medium">人工复核</h3>
            <p className="text-xs text-muted-foreground">判断有偏差时，可补充独立标签和理由。</p>
          </div>
          <Button
            variant="outline"
            aria-expanded={correcting}
            aria-controls={`${disclosureID}-correction`}
            onClick={() => setCorrecting(!correcting)}
          >
            人工纠正
          </Button>
        </div>
        {query.data.annotation && !correcting ? (
          <p className="mt-3 text-xs text-muted-foreground">
            已标注：{animationQualityLabels[query.data.annotation.label]}
          </p>
        ) : null}
        {correcting ? (
          <div id={`${disclosureID}-correction`} className="mt-4 border-t pt-4">
            <QualityForm
              key={`${query.data.sample_id}:${query.data.annotation?.version ?? 0}`}
              review={query.data}
              result={props.result}
              taskID={props.taskID}
            />
          </div>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button
          variant="ghost"
          aria-expanded={samplesOpen}
          aria-controls={`${disclosureID}-samples`}
          onClick={() => setSamplesOpen(!samplesOpen)}
        >
          样本管理
        </Button>
        <Button variant="ghost" onClick={() => void query.refetch()} disabled={query.isFetching}>
          重新读取标注
        </Button>
      </div>
      {samplesOpen ? (
        <div id={`${disclosureID}-samples`} className="rounded-xl border bg-background p-4">
          <AnimationQualityExport />
        </div>
      ) : null}
    </section>
  );
}
function QualityForm(props: {
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
