import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { RefreshCw, Save } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { notifyOperationError } from "@/lib/operation-feedback";
import { templateKeys, workbenchKeys } from "../constants";
import { templateSchema, type TemplateValues } from "../lib/template-schema";
import type { WorkbenchTemplate } from "../types";
import { TemplateDetails } from "./template-details";

export function TemplateEditor(props: {
  template?: WorkbenchTemplate;
  revision: number;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const form = useForm<TemplateValues>({
    resolver: zodResolver(templateSchema),
    defaultValues: { source_id: props.template?.source_id ?? "", name: props.template?.name ?? "" },
  });
  const sourceID = form.watch("source_id");
  const [readID, setReadID] = useState(props.template?.source_id ?? "");
  const accounts = useQuery({ queryKey: workbenchKeys.accounts, queryFn: api.workbenchAccounts });
  const source = useQuery({
    queryKey: ["account-workbench", "template-source", readID],
    queryFn: () => api.workbenchTemplateSource(readID),
    enabled: !!readID,
    staleTime: 0,
    gcTime: 0,
  });
  const save = useMutation({
    mutationFn: (values: TemplateValues) =>
      api.saveWorkbenchTemplate({
        ...values,
        id: props.template?.id,
        source_version: source.data!.source_version,
        revision: props.revision,
      }),
    onSuccess: (result) => {
      client.setQueryData(templateKeys.library, result);
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "模板保存失败"),
  });
  const submit = (values: TemplateValues): void => {
    if (source.data && readID === values.source_id && !source.isFetching && !source.isError)
      save.mutate(values);
  };
  const ready = !!source.data && readID === sourceID && !source.isFetching && !source.isError;
  const sourceAccount = accounts.data?.find((account) => account.id === sourceID);
  const sourceName =
    sourceAccount?.name ||
    sourceAccount?.email ||
    props.template?.source_name ||
    (sourceID ? `账号 #${sourceID}` : "");
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose();
      }}
    >
      <DialogContent width="progress" render={<form onSubmit={form.handleSubmit(submit)} />}>
        <DialogHeader>
          <DialogTitle>{props.template ? "重新同步模板" : "创建配置模板"}</DialogTitle>
          <DialogDescription>从来源账号读取配置，确认后保存为导入模板。</DialogDescription>
        </DialogHeader>
        <DialogBody className="grid content-start gap-5">
          <div className="grid gap-4 rounded-lg border bg-muted/20 p-3 sm:p-4">
            <div className="grid gap-2">
              <label htmlFor="workbench-template-source" className="text-sm">
                来源账号
              </label>
              <Controller
                name="source_id"
                control={form.control}
                render={({ field }) => (
                  <Select
                    value={field.value}
                    disabled={save.isPending || !!props.template}
                    onValueChange={(value) => {
                      field.onChange(value ?? "");
                      setReadID("");
                    }}
                  >
                    <SelectTrigger id="workbench-template-source" aria-label="选择模板来源账号">
                      <SelectValue placeholder="选择来源账号">
                        {sourceName || undefined}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {accounts.data?.map((account) => (
                        <SelectItem key={account.id} value={account.id}>
                          {account.name || account.email || `账号 #${account.id}`} · #{account.id}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              {accounts.isPending && <ContentLoading compact label="正在读取账号" />}
              {accounts.isError && !accounts.data && (
                <ContentRetry
                  pending={accounts.isFetching}
                  onRetry={() => void accounts.refetch()}
                />
              )}
            </div>
            <div className="grid gap-2">
              <label htmlFor="workbench-template-name" className="text-sm">
                模板名称
              </label>
              <Input
                id="workbench-template-name"
                disabled={save.isPending}
                aria-invalid={!!form.formState.errors.name}
                {...form.register("name")}
              />
              {form.formState.errors.name && (
                <p role="alert" className="text-sm text-destructive">
                  {form.formState.errors.name.message}
                </p>
              )}
            </div>
            <Button
              type="button"
              variant="outline"
              className="justify-self-start"
              disabled={!sourceID || source.isFetching || save.isPending}
              onClick={() => {
                if (readID === sourceID) void source.refetch();
                else setReadID(sourceID);
              }}
            >
              <RefreshCw aria-hidden="true" />
              读取配置
            </Button>
          </div>
          {readID && source.isFetching && <ContentLoading label="正在读取来源配置" />}
          {readID && source.isError && (
            <ContentRetry pending={source.isFetching} onRetry={() => void source.refetch()} />
          )}
          {ready && source.data && (
            <section aria-label="配置预览" className="min-w-0 space-y-3">
              <h3 className="text-sm font-medium">配置预览</h3>
              <TemplateDetails template={source.data} />
            </section>
          )}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" disabled={save.isPending} onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" disabled={!ready || save.isPending}>
            <Save aria-hidden="true" />
            {save.isPending ? "正在保存" : "保存模板"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
