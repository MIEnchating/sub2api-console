import { useId, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  api,
  type DetectionTimeout,
  type DetectionTimeoutMode,
  type AnimationQualitySettings,
} from "@/api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { notifyOperationError } from "@/lib/operation-feedback";
import { detectionTimeoutSchema, type DetectionTimeoutForm } from "../lib/detection-timeout-schema";
import { detectionTimeoutLabels } from "../constants";

type Props = {
  mode: DetectionTimeoutMode;
  disabled?: boolean;
  onSaved?: (value: DetectionTimeout) => void;
};
export function SavedDetectionTimeout(props: Props): ReactElement {
  const query = useQuery({
    queryKey: ["detection-timeout", props.mode],
    queryFn: ({ signal }) => api.detectionTimeout(props.mode, signal),
  });
  if (!query.data)
    return (
      <div className="min-w-0">
        {query.isError ? (
          <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />
        ) : (
          <ContentLoading compact label={`正在读取${detectionTimeoutLabels[props.mode]}`} />
        )}
      </div>
    );
  return (
    <TimeoutEditor key={`${props.mode}:${query.data.version}`} {...props} value={query.data} />
  );
}
function TimeoutEditor(props: Props & { value: DetectionTimeout }): ReactElement {
  const id = useId();
  const client = useQueryClient();
  const form = useForm<DetectionTimeoutForm>({
    resolver: zodResolver(detectionTimeoutSchema),
    defaultValues: { timeout_seconds: props.value.timeout_seconds },
  });
  const save = useMutation({
    mutationFn: (value: DetectionTimeoutForm) =>
      api.saveDetectionTimeout(props.mode, { ...value, version: props.value.version }),
    onSuccess: (value) => {
      client.setQueryData(["detection-timeout", props.mode], value);
      if (props.mode === "review")
        client.setQueryData<AnimationQualitySettings>(["animation-quality-settings"], (previous) =>
          previous ? { ...previous, ...value } : previous,
        );
      props.onSaved?.(value);
      toast.success(`${detectionTimeoutLabels[props.mode]}已保存`);
    },
    onError: (error) => notifyOperationError(error, "超时保存失败"),
  });
  const label = detectionTimeoutLabels[props.mode];
  return (
    <div role="group" aria-label={`${label}设置`} className="min-w-0 space-y-1.5">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <label htmlFor={id} className="shrink-0 text-xs font-medium">
          {label}（秒）
        </label>
        <Input
          id={id}
          className="w-20"
          type="number"
          min={5}
          max={120}
          {...form.register("timeout_seconds", { valueAsNumber: true })}
          disabled={props.disabled || save.isPending}
          aria-invalid={!!form.formState.errors.timeout_seconds}
          aria-describedby={form.formState.errors.timeout_seconds ? `${id}-error` : undefined}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              if (!props.disabled && !save.isPending && form.formState.isDirty)
                void form.handleSubmit((value) => save.mutate(value))();
            }
          }}
        />
        <Button
          type="button"
          variant="outline"
          aria-label={`保存${label}`}
          disabled={props.disabled || save.isPending || !form.formState.isDirty}
          onClick={() => void form.handleSubmit((value) => save.mutate(value))()}
        >
          {save.isPending ? "保存中" : "保存"}
        </Button>
        {save.isError ? (
          <Button
            type="button"
            variant="ghost"
            aria-label={`重新读取${label}`}
            disabled={props.disabled || save.isPending}
            onClick={() =>
              void client.invalidateQueries({ queryKey: ["detection-timeout", props.mode] })
            }
          >
            重新读取
          </Button>
        ) : null}
      </div>
      {form.formState.errors.timeout_seconds ? (
        <p id={`${id}-error`} className="text-xs text-destructive">
          {form.formState.errors.timeout_seconds.message}
        </p>
      ) : null}
    </div>
  );
}
