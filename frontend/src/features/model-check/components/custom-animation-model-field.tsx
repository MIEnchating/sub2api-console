import { DetectionModelSelect } from "./detection-model-select";
import { useQuery } from "@tanstack/react-query";
import { LoaderCircle, RefreshCw } from "lucide-react";
import { useMemo, useState, type ReactElement } from "react";
import { Controller, useWatch, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { CustomAnimationForm } from "../lib/animation-schema";

const emptyModels: string[] = [];

export function CustomAnimationModelField(props: {
  form: UseFormReturn<CustomAnimationForm>;
  disabled: boolean;
}): ReactElement {
  const [baseURL, apiKey, platform] = useWatch({
    control: props.form.control,
    name: ["base_url", "api_key", "platform"],
  });
  // An opaque query identity keeps credentials out of the cache key and isolates late responses.
  const connection = useMemo(
    () => ({
      id: crypto.getRandomValues(new Uint32Array(4)).join("-"),
      payload: { base_url: baseURL.trim(), api_key: apiKey.trim(), platform },
    }),
    [baseURL, apiKey, platform],
  );
  const models = useQuery({
    queryKey: ["model-animation", "custom-models", connection.id],
    queryFn: ({ signal }) => api.customAnimationModels(connection.payload, signal),
    enabled: false,
    retry: false,
    gcTime: 0,
    staleTime: Infinity,
  });
  const [open, setOpen] = useState(false);
  const load = async (): Promise<void> => {
    if (!(await props.form.trigger(["base_url", "api_key", "platform"]))) return;
    const result = await models.refetch();
    if (result.isSuccess) {
      setOpen(true);
    }
  };
  return (
    <div className="min-w-0 space-y-2">
      <Controller
        control={props.form.control}
        name="model"
        render={({ field, fieldState }) => (
          <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5">
            <label
              htmlFor="custom-animation-model"
              className="whitespace-nowrap text-xs font-medium"
            >
              检测模型
            </label>
            <div className="flex min-w-0 items-center gap-2">
              <DetectionModelSelect
                id="custom-animation-model"
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
                options={models.data?.models ?? emptyModels}
                disabled={props.disabled}
                invalid={!!fieldState.error}
                describedBy={fieldState.error ? "custom-animation-model-error" : undefined}
                open={open}
                onOpenChange={setOpen}
              />
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      disabled={props.disabled || models.isFetching}
                      aria-label="获取模型"
                      aria-busy={models.isFetching}
                      onClick={() => void load()}
                    />
                  }
                >
                  {models.isFetching ? (
                    <span role="status" aria-label="正在读取模型">
                      <LoaderCircle aria-hidden="true" className="animate-spin" />
                    </span>
                  ) : (
                    <RefreshCw aria-hidden="true" />
                  )}
                </TooltipTrigger>
                <TooltipContent>获取模型</TooltipContent>
              </Tooltip>
            </div>
            {fieldState.error ? (
              <FieldError
                className="col-start-2"
                id="custom-animation-model-error"
                message={fieldState.error.message}
              />
            ) : null}
          </div>
        )}
      />
    </div>
  );
}
