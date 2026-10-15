import { DetectionModelSelect } from "./detection-model-select";
import { useQueries } from "@tanstack/react-query";
import { useId, useState, type ReactElement } from "react";
import { Controller, useFormState, useWatch, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { SavedDetectionTimeout } from "./saved-detection-timeout";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { toast } from "sonner";
import { LoaderCircle, RefreshCw } from "lucide-react";
import type { AnimationForm } from "../lib/animation-schema";
import { createQueuedRead } from "../lib/queued-read";

export function AnimationModelSettings(props: {
  form: UseFormReturn<AnimationForm>;
  pending: boolean;
  mode?: "animation" | "precheck";
}): ReactElement {
  const id = useId();
  const modelID = id + "-model";
  const modelErrorID = id + "-model-error";

  const selected = useWatch({ control: props.form.control, name: "account_ids", exact: true });
  const state = useFormState({
    control: props.form.control,
    name: ["account_ids", "unified_model"],
    exact: true,
  });
  const [requested, setRequested] = useState(false);
  const [readModels] = useState(() => createQueuedRead(api.accountAnimationModels));
  const queries = useQueries({
    queries: selected.map((id) => ({
      queryKey: ["model-animation", "account-models", id],
      queryFn: ({ signal }: { signal: AbortSignal }) => readModels(id, signal),
      enabled: requested,
      staleTime: 0,
      gcTime: 0,
      refetchOnWindowFocus: false,
      retry: false,
    })),
  });
  const loading = queries.some((query) => query.isFetching);
  const ready =
    requested &&
    queries.length > 0 &&
    queries.every((query) => query.isSuccess && !query.isFetching);
  const lists = queries.map((query) => query.data?.models);
  const remaining = lists.slice(1).map((list) => new Set(list));
  const options = ready
    ? [...new Set(lists[0] ?? [])]
        .filter((model) => remaining.every((list) => list.has(model)))
        .sort()
    : [];
  return (
    <div
      role="group"
      aria-label="动画模型参数"
      className="grid w-full max-w-full min-w-0 shrink-0 grid-cols-1 items-start gap-3 sm:w-[37rem] sm:grid-cols-[minmax(0,1fr)_16rem]"
    >
      <div
        role="group"
        aria-label="检测模型设置"
        className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5"
      >
        <label
          id={modelID + "-label"}
          htmlFor={modelID}
          className="shrink-0 whitespace-nowrap text-xs font-medium"
        >
          检测模型
        </label>
        <div className="flex min-w-0 gap-2">
          <Controller
            control={props.form.control}
            name="unified_model"
            render={({ field }) => (
              <DetectionModelSelect
                id={modelID}
                inputRef={field.ref}
                model={field.value}
                additionalModels={props.form.watch("additional_models")}
                onChange={(model, additional) => {
                  field.onChange(model);
                  props.form.setValue("additional_models", additional, {
                    shouldDirty: true,
                    shouldValidate: true,
                  });
                }}
                onBlur={field.onBlur}
                options={options}
                disabled={props.pending}
                invalid={!!state.errors.unified_model}
                describedBy={state.errors.unified_model ? modelErrorID : undefined}
              />
            )}
          />
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="获取模型"
                  aria-busy={loading}
                  disabled={props.pending || !selected.length || loading}
                  onClick={() => {
                    if (!selected.length) {
                      toast.error("请先选择至少一个账号");
                      return;
                    }
                    setRequested(true);
                    if (requested) void Promise.all(queries.map((query) => query.refetch()));
                  }}
                />
              }
            >
              {loading ? (
                <span role="status" aria-label="正在读取共同模型">
                  <LoaderCircle aria-hidden="true" className="animate-spin" />
                </span>
              ) : (
                <RefreshCw aria-hidden="true" />
              )}
            </TooltipTrigger>
            <TooltipContent>获取模型</TooltipContent>
          </Tooltip>
        </div>
        {state.errors.unified_model ? (
          <FieldError
            className="col-start-2"
            id={modelErrorID}
            message={state.errors.unified_model.message}
          />
        ) : null}
      </div>
      <SavedDetectionTimeout mode={props.mode ?? "animation"} disabled={props.pending} />
    </div>
  );
}
