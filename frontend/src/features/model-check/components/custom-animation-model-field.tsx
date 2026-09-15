import { Autocomplete } from "@base-ui/react/autocomplete";
import { useQuery } from "@tanstack/react-query";
import { LoaderCircle, RefreshCw } from "lucide-react";
import { useMemo, useRef, useState, type ReactElement } from "react";
import { Controller, useWatch, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  const anchor = useRef<HTMLDivElement>(null);
  const load = async (): Promise<void> => {
    if (!(await props.form.trigger(["base_url", "api_key", "platform"]))) return;
    const result = await models.refetch();
    if (result.isSuccess) {
      setOpen(true);
      props.form.setFocus("model");
    }
  };
  return (
    <Controller
      control={props.form.control}
      name="model"
      render={({ field, fieldState }) => (
        <div className="col-span-2 min-w-0 space-y-1">
          <label htmlFor="custom-animation-model" className="text-sm font-medium">
            检测模型
          </label>
          <Autocomplete.Root
            items={models.data?.models ?? emptyModels}
            value={field.value}
            onValueChange={field.onChange}
            open={open && models.data !== undefined && !props.disabled}
            onOpenChange={setOpen}
            disabled={props.disabled}
          >
            <div className="flex min-w-0 items-center gap-2">
              <div ref={anchor} className="min-w-0 flex-1">
                <Autocomplete.Input
                  id="custom-animation-model"
                  name={field.name}
                  ref={field.ref}
                  onBlur={field.onBlur}
                  placeholder="输入或选择模型 ID"
                  aria-invalid={!!fieldState.error}
                  aria-describedby={fieldState.error ? "custom-animation-model-error" : undefined}
                  render={<Input autoComplete="off" spellCheck={false} />}
                />
              </div>
              <Button
                type="button"
                variant="outline"
                disabled={props.disabled || models.isFetching}
                aria-label="获取模型"
                aria-busy={models.isFetching}
                onClick={() => void load()}
              >
                {models.isFetching ? (
                  <span role="status" aria-label="正在读取模型">
                    <LoaderCircle aria-hidden="true" className="animate-spin" />
                  </span>
                ) : (
                  <RefreshCw aria-hidden="true" />
                )}
                获取模型
              </Button>
            </div>
            <Autocomplete.Portal>
              <Autocomplete.Positioner anchor={anchor} sideOffset={4} className="z-50">
                <Autocomplete.Popup
                  initialFocus={false}
                  className="max-h-(--available-height) w-(--anchor-width) max-w-(--available-width) overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-md"
                >
                  <Autocomplete.Empty className="p-3 text-sm text-muted-foreground empty:p-0">
                    暂无匹配模型
                  </Autocomplete.Empty>
                  <Autocomplete.List className="max-h-64 overflow-y-auto overscroll-contain p-1">
                    {(model: string) => (
                      <Autocomplete.Item
                        key={model}
                        value={model}
                        className="cursor-default rounded-md px-2 py-1.5 text-sm break-all data-highlighted:bg-accent data-highlighted:text-accent-foreground"
                      >
                        {model}
                      </Autocomplete.Item>
                    )}
                  </Autocomplete.List>
                </Autocomplete.Popup>
              </Autocomplete.Positioner>
            </Autocomplete.Portal>
          </Autocomplete.Root>
          {fieldState.error ? (
            <FieldError id="custom-animation-model-error" message={fieldState.error.message} />
          ) : null}
        </div>
      )}
    />
  );
}
