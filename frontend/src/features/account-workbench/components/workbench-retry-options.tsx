import type { ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { RotateCw } from "lucide-react";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { workbenchKeys } from "../constants";
import { retrySchema, type RetryValues } from "../lib/schemas";

export function WorkbenchRetryOptions(props: {
  disabled: boolean;
  onChange: () => void;
  onSubmit: (value: RetryValues) => void;
}): ReactElement {
  const templates = useQuery({
    queryKey: workbenchKeys.templates,
    queryFn: api.workbenchTemplates,
  });
  const form = useForm<RetryValues>({
    resolver: zodResolver(retrySchema),
    defaultValues: { template_id: "", model: "" },
  });
  if (templates.isPending) return <ContentLoading label="正在读取重试配置" />;
  if (!templates.data)
    return <ContentRetry pending={templates.isFetching} onRetry={() => void templates.refetch()} />;
  return (
    <form
      className="grid min-w-0 gap-3"
      onChange={props.onChange}
      onSubmit={form.handleSubmit(props.onSubmit)}
    >
      <div className="grid min-w-0 gap-3 sm:grid-cols-2">
        <FormField label="重试模板">
          <Controller
            control={form.control}
            name="template_id"
            render={({ field }) => (
              <Select
                value={field.value || "original"}
                disabled={props.disabled}
                onValueChange={(value) => {
                  props.onChange();
                  field.onChange(value === "original" ? "" : value);
                }}
              >
                <SelectTrigger aria-label="重试模板">
                  <SelectValue>
                    {templates.data.find((item) => item.id === field.value)?.name ||
                      "原任务模板的最新版本"}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="original">原任务模板的最新版本</SelectItem>
                  {templates.data.map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
        <FormField
          label="重试检测模型"
          htmlFor="workbench-retry-model"
          error={form.formState.errors.model?.message}
        >
          <Input
            id="workbench-retry-model"
            disabled={props.disabled}
            aria-invalid={!!form.formState.errors.model}
            {...form.register("model")}
          />
        </FormField>
      </div>
      <div>
        <Button type="submit" disabled={props.disabled}>
          <RotateCw aria-hidden="true" />
          预览重新处理范围
        </Button>
      </div>
    </form>
  );
}
