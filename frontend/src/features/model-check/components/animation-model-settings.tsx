import { useQueries } from "@tanstack/react-query";
import { useState, type ReactElement } from "react";
import { Controller, useFormState, useWatch, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { FieldError } from "@/components/field-error";
import { SuggestionInput } from "@/components/suggestion-input";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { toast } from "sonner";
import { LoaderCircle, RefreshCw } from "lucide-react";
import type { AnimationForm } from "../lib/animation-schema";

export function AnimationModelSettings(props: {
  form: UseFormReturn<AnimationForm>;
  pending: boolean;
}): ReactElement {
  const selected = useWatch({ control: props.form.control, name: "account_ids", exact: true });
  const state = useFormState({
    control: props.form.control,
    name: ["account_ids", "unified_model", "timeout_seconds"],
    exact: true,
  });
  const [requested, setRequested] = useState(false);
  const queries = useQueries({
    queries: selected.map((id) => ({
      queryKey: ["model-animation", "account-models", id],
      queryFn: ({ signal }: { signal: AbortSignal }) => api.accountAnimationModels(id, signal),
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
      className="grid w-full max-w-full min-w-0 shrink-0 grid-cols-1 items-start gap-3 sm:w-[36rem] sm:grid-cols-[minmax(0,1fr)_12rem]"
    >
      <div
        role="group"
        aria-label="检测模型设置"
        className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5"
      >
        <label
          id="animation-unified-model-label"
          htmlFor="animation-unified-model"
          className="shrink-0 whitespace-nowrap text-xs font-medium"
        >
          检测模型
        </label>
        <div className="flex min-w-0 gap-2">
          <Controller
            control={props.form.control}
            name="unified_model"
            render={({ field }) => (
              <SuggestionInput
                className="flex-1"
                id="animation-unified-model"
                aria-labelledby="animation-unified-model-label"
                ref={field.ref}
                name={field.name}
                value={field.value}
                onValueChange={field.onChange}
                onBlur={field.onBlur}
                options={options}
                disabled={props.pending}
                placeholder="输入模型 ID，或获取共同模型"
                emptyText={ready ? "所选账号暂无共同模型" : "尚未获取共同模型"}
                aria-invalid={!!state.errors.unified_model}
                aria-describedby={
                  state.errors.unified_model ? "animation-unified-model-error" : undefined
                }
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
            id="animation-unified-model-error"
            message={state.errors.unified_model.message}
          />
        ) : null}
      </div>
      <div
        role="group"
        aria-label="请求超时设置"
        className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5"
      >
        <label
          htmlFor="animation-timeout"
          className="shrink-0 whitespace-nowrap text-xs font-medium"
        >
          请求超时（秒）
        </label>
        <Input
          className="w-full"
          id="animation-timeout"
          type="number"
          min={5}
          max={120}
          {...props.form.register("timeout_seconds", { valueAsNumber: true })}
          disabled={props.pending}
          aria-invalid={!!state.errors.timeout_seconds}
          aria-describedby={state.errors.timeout_seconds ? "animation-timeout-error" : undefined}
        />
        <FieldError
          className="col-start-2"
          id="animation-timeout-error"
          message={state.errors.timeout_seconds?.message}
        />
      </div>
    </div>
  );
}
