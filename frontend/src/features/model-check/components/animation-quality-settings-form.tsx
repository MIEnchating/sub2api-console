import { useId, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type AnimationQualitySettings } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
      toast.success("自动质量标注设置已保存");
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "自动质量标注设置保存失败"),
  });
  return (
    <form
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
              <select
                id={`${id}-account`}
                {...form.register("account_id")}
                aria-invalid={!!form.formState.errors.account_id}
                aria-describedby={`${id}-account-error`}
                className="h-8 w-full rounded-md border bg-background px-2 text-sm"
              >
                <option value="">选择已有账号</option>
                {accountID && !accounts.data.some((account) => account.id === accountID) ? (
                  <option value={accountID} disabled>
                    原账号已不可用（{accountID}）
                  </option>
                ) : null}
                {accounts.data.map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.name} · {account.id}
                  </option>
                ))}
              </select>
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
              <Input
                id={`${id}-model`}
                list={`${id}-models`}
                {...form.register("model")}
                placeholder="选择或输入模型 ID"
                aria-invalid={!!form.formState.errors.model}
                aria-describedby={`${id}-model-error`}
              />
              <Button
                type="button"
                variant="outline"
                disabled={!accountID || models.isFetching || save.isPending || !enabled}
                onClick={() => void models.refetch()}
              >
                {models.isFetching ? "读取中" : "获取模型"}
              </Button>
              <datalist id={`${id}-models`}>
                {models.data?.models.map((model) => (
                  <option key={model} value={model} />
                ))}
              </datalist>
            </div>
            <p id={`${id}-model-error`} className="text-xs text-destructive">
              {form.formState.errors.model?.message}
            </p>
          </div>
          <div className="space-y-1.5">
            <label htmlFor={`${id}-timeout`}>评审超时（秒）</label>
            <Input
              id={`${id}-timeout`}
              type="number"
              min={5}
              max={120}
              {...form.register("timeout_seconds", { valueAsNumber: true })}
              aria-invalid={!!form.formState.errors.timeout_seconds}
              aria-describedby={`${id}-timeout-error`}
            />
            <p id={`${id}-timeout-error`} className="text-xs text-destructive">
              {form.formState.errors.timeout_seconds?.message}
            </p>
          </div>
        </fieldset>
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
