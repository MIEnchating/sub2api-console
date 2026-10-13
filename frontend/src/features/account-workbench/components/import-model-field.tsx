import { useEffect, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { UseFormReturn } from "react-hook-form";
import { toast } from "sonner";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { notifyOperationError } from "@/lib/operation-feedback";
import { importModelKey } from "../constants";
import type { ImportValues } from "../lib/import-schema";
import type { ImportModelSettings } from "../types";

export function ImportModelField(props: {
  form: UseFormReturn<ImportValues>;
  busy: boolean;
  onReadyChange: (ready: boolean) => void;
}): ReactElement {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: importModelKey,
    queryFn: ({ signal }) => api.workbenchImportModel(signal),
  });
  const dirty = !!props.form.formState.dirtyFields.model;
  const check = props.form.watch("check");
  const error = props.form.formState.errors.model;
  useEffect(() => {
    if (query.data && !dirty) props.form.resetField("model", { defaultValue: query.data.model });
  }, [query.data, dirty, props.form]);
  const save = useMutation({
    mutationFn: (value: ImportModelSettings) => api.saveWorkbenchImportModel(value),
    onSuccess: async (value) => {
      await client.cancelQueries({ queryKey: importModelKey });
      client.setQueryData(importModelKey, value);
      props.form.resetField("model", { defaultValue: value.model });
      toast.success("检测模型已保存");
    },
    onError: (cause) => {
      notifyOperationError(cause, "检测模型保存失败");
      void client.invalidateQueries({ queryKey: importModelKey });
    },
  });
  const ready = !!query.data && !save.isPending;
  useEffect(() => {
    props.onReadyChange(ready);
  }, [ready, props.onReadyChange]);
  async function saveModel(): Promise<void> {
    if (!query.data || props.busy || save.isPending) return;
    if (await props.form.trigger("model")) {
      save.mutate({ model: props.form.getValues("model").trim(), revision: query.data.revision });
    }
  }
  return (
    <div className="grid min-w-0 gap-2">
      <label htmlFor="workbench-model" className="text-sm">
        检测模型
      </label>
      <div className="flex min-w-0 items-center gap-2">
        <Input
          id="workbench-model"
          className="min-w-0 flex-1"
          disabled={props.busy || !check || !ready}
          aria-invalid={!!error}
          aria-describedby={error ? "workbench-model-error" : undefined}
          {...props.form.register("model")}
        />
        <Button
          type="button"
          variant="outline"
          aria-label="保存检测模型"
          disabled={props.busy || !check || !ready}
          onClick={() => void saveModel()}
        >
          {save.isPending ? "保存中…" : "保存"}
        </Button>
      </div>
      {query.isPending && <ContentLoading compact label="正在读取检测模型" />}
      {query.isError && !query.data && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {error && (
        <p id="workbench-model-error" role="alert" className="text-sm text-destructive">
          {error.message}
        </p>
      )}
    </div>
  );
}
