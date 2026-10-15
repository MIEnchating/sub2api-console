import { useId, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type AnimationQualitySettings } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { SavedDetectionTimeout } from "./saved-detection-timeout";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { DialogBody, DialogFooter } from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import {
  qualitySettingsSchema,
  type QualitySettingsForm,
} from "../lib/animation-quality-settings-schema";

export function QualitySettingsFormEditor(props: {
  value: AnimationQualitySettings;
  onClose: () => void;
}): ReactElement {
  const id = useId();
  const client = useQueryClient();
  const form = useForm<QualitySettingsForm>({
    resolver: zodResolver(qualitySettingsSchema),
    defaultValues: props.value,
  });
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: ({ signal }) => api.accounts(signal),
  });
  const enabled = form.watch("enabled");
  const accountID = form.watch("account_id");
  const models = useQuery({
    queryKey: ["model-animation", "account-models", accountID],
    queryFn: ({ signal }) => api.accountAnimationModels(accountID, signal),
    enabled: false,
    retry: false,
  });
  const save = useMutation({
    mutationFn: api.saveAnimationQualitySettings,
    onSuccess: (value) => {
      client.setQueryData(["animation-quality-settings"], value);
      client.setQueryData(["detection-timeout", "review"], {
        timeout_seconds: value.timeout_seconds,
        version: value.version,
      });
      toast.success("自动质量标注设置已保存");
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "自动质量标注设置保存失败"),
  });
  return (
    <form
      noValidate
      onSubmit={form.handleSubmit((value) => save.mutate(value))}
      className="flex min-h-0 flex-col"
    >
      <DialogBody className="space-y-4">
        <label className="flex items-center gap-2 rounded-lg border bg-muted/30 p-3">
          <input type="checkbox" {...form.register("enabled")} disabled={save.isPending} />
          生成后自动标注质量
        </label>
        <fieldset disabled={save.isPending || !enabled} className="space-y-4 disabled:opacity-60">
          <div className="space-y-1.5">
            <label htmlFor={`${id}-account`}>评审账号</label>
            {accounts.data ? (
              <Controller
                control={form.control}
                name="account_id"
                render={({ field }) => (
                  <Select
                    value={field.value || null}
                    onValueChange={(value) => {
                      field.onChange(value ?? "");
                      form.setValue("model", "", { shouldDirty: true });
                    }}
                    disabled={save.isPending || !enabled}
                  >
                    <SelectTrigger
                      id={`${id}-account`}
                      ref={field.ref}
                      onBlur={field.onBlur}
                      aria-invalid={!!form.formState.errors.account_id}
                      aria-describedby={`${id}-account-error`}
                    >
                      <SelectValue placeholder="选择已有账号">
                        {accounts.data?.find((account) => account.id === field.value)?.name ??
                          (field.value ? `原账号已不可用（${field.value}）` : "选择已有账号")}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent className="max-h-72">
                      <SelectItem value="">清除选择</SelectItem>
                      {accounts.data?.map((account) => (
                        <SelectItem key={account.id} value={account.id}>
                          {account.name} · {account.id}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            ) : (
              <div>
                {accounts.isError ? (
                  <ContentRetry
                    onRetry={() => void accounts.refetch()}
                    pending={accounts.isFetching}
                  />
                ) : (
                  <ContentLoading compact label="正在读取评审账号" />
                )}
              </div>
            )}
            <p id={`${id}-account-error`} className="text-xs text-destructive">
              {form.formState.errors.account_id?.message}
            </p>
          </div>
          <div className="space-y-1.5">
            <label htmlFor={`${id}-model`}>评审模型</label>
            <div className="flex min-w-0 gap-2">
              <Controller
                control={form.control}
                name="model"
                render={({ field }) => (
                  <Select
                    value={field.value || null}
                    onValueChange={(value) => field.onChange(value ?? "")}
                    disabled={
                      !accountID || !models.data?.models.length || save.isPending || !enabled
                    }
                  >
                    <SelectTrigger
                      id={`${id}-model`}
                      className="min-w-0 flex-1"
                      ref={field.ref}
                      onBlur={field.onBlur}
                      aria-invalid={!!form.formState.errors.model}
                      aria-describedby={`${id}-model-error`}
                    >
                      <SelectValue placeholder="获取模型后选择评审模型">
                        {field.value || "获取模型后选择评审模型"}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent className="max-h-72">
                      {models.data?.models.map((model) => (
                        <SelectItem key={model} value={model}>
                          {model}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              <Button
                type="button"
                variant="outline"
                disabled={!accountID || models.isFetching || save.isPending || !enabled}
                onClick={() => void models.refetch()}
              >
                {models.isFetching ? "读取中" : "获取模型"}
              </Button>
            </div>
            {models.data?.models.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                该账号未返回可选模型，可重新获取或更换账号。
              </p>
            ) : null}
            <p id={`${id}-model-error`} className="text-xs text-destructive">
              {form.formState.errors.model?.message}
            </p>
          </div>
        </fieldset>
        <SavedDetectionTimeout
          mode="review"
          disabled={save.isPending}
          onSaved={(value) => {
            form.setValue("timeout_seconds", value.timeout_seconds);
            form.setValue("version", value.version);
          }}
        />
        <p className="text-xs leading-6 text-muted-foreground">
          评审只分析源码，不运行或观看动画。自动结果与人工标注分别保存；历史记录不会自动重评。设置变更会中止尚未提交的旧配置评审。
        </p>
      </DialogBody>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={props.onClose}>
          取消
        </Button>
        <Button type="submit" disabled={save.isPending || (enabled && !accounts.data)}>
          {save.isPending ? "正在保存" : "保存设置"}
        </Button>
      </DialogFooter>
    </form>
  );
}
