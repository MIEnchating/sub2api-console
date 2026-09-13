import { useQueries } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode, type ReactElement } from "react";
import { useFormState, useWatch, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { toast } from "sonner";
import type { AnimationForm } from "../lib/animation-schema";

export function AnimationModelSettings(props: {
  form: UseFormReturn<AnimationForm>;
  pending: boolean;
  children?: ReactNode;
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
      queryKey: ["model-check-account-models", id],
      queryFn: () => api.accountModels(id),
      enabled: requested,
      staleTime: 300_000,
      retry: false,
    })),
  });
  const loading = queries.some((query) => query.isFetching);
  const failedQuery = queries.find((query) => query.isError);
  const failedMessage = failedQuery?.error instanceof Error ? failedQuery.error.message : null;
  useEffect(() => {
    if (failedMessage) toast.error(`模型列表读取失败：${failedMessage}`);
  }, [failedMessage]);
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
    <div className="relative min-w-0">
      <div className="overflow-x-auto">
        <div className="flex w-max min-w-full items-center gap-3">
          <div className="flex items-center gap-2">
            <label
              htmlFor="animation-unified-model"
              className="shrink-0 whitespace-nowrap text-xs font-medium"
            >
              检测模型
            </label>
            <div className="flex gap-2">
              <Input
                className="w-56"
                id="animation-unified-model"
                {...props.form.register("unified_model")}
                list="animation-common-models"
                disabled={props.pending}
                placeholder="输入模型 ID，或获取共同模型"
                aria-invalid={!!state.errors.unified_model}
                aria-describedby={
                  state.errors.unified_model ? "animation-unified-model-error" : undefined
                }
              />
              <Button
                type="button"
                variant="outline"
                disabled={props.pending || !selected.length || loading}
                onClick={() => {
                  if (!selected.length) {
                    toast.error("请先选择至少一个账号");
                    return;
                  }
                  setRequested(true);
                  if (requested) void Promise.all(queries.map((query) => query.refetch()));
                }}
              >
                获取模型
              </Button>
            </div>
            <datalist id="animation-common-models">
              {options.map((model) => (
                <option key={model} value={model} />
              ))}
            </datalist>
          </div>
          <div className="flex items-center gap-2">
            <label
              htmlFor="animation-timeout"
              className="shrink-0 whitespace-nowrap text-xs font-medium"
            >
              请求超时（秒）
            </label>
            <Input
              className="w-20"
              id="animation-timeout"
              type="number"
              min={5}
              max={120}
              {...props.form.register("timeout_seconds", { valueAsNumber: true })}
              disabled={props.pending}
              aria-invalid={!!state.errors.timeout_seconds}
              aria-describedby={
                state.errors.timeout_seconds ? "animation-timeout-error" : undefined
              }
            />
          </div>
          <div className="ml-auto flex items-center gap-2 border-l pl-3">{props.children}</div>
        </div>
      </div>
    </div>
  );
}
