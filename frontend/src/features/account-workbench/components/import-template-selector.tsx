import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { Controller, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { templateKeys } from "../constants";
import type { ImportValues } from "../lib/import-schema";

export function ImportTemplateSelector(props: {
  form: UseFormReturn<ImportValues>;
  busy: boolean;
}): ReactElement {
  const templates = useQuery({
    queryKey: templateKeys.library,
    queryFn: ({ signal }) => api.workbenchTemplates(signal),
  });
  return (
    <div className="grid min-w-0 grid-cols-1 gap-2">
      <label htmlFor="workbench-template" className="text-sm">
        配置模板
      </label>
      <Controller
        name="template_id"
        control={props.form.control}
        render={({ field }) => (
          <Select
            value={field.value || "__auto__"}
            disabled={props.busy || !templates.data}
            onValueChange={(value) => field.onChange(value === "__auto__" ? "" : value)}
          >
            <SelectTrigger id="workbench-template">
              <SelectValue>
                {templates.data?.items.find((item) => item.id === field.value)?.name ||
                  "首选模板 / 自动匹配"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__auto__">首选模板 / 自动匹配</SelectItem>
              {templates.data?.items.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {item.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
      {templates.isPending && <ContentLoading compact label="正在读取配置模板" />}
      {templates.isError && !templates.data && (
        <ContentRetry pending={templates.isFetching} onRetry={() => void templates.refetch()} />
      )}
    </div>
  );
}
