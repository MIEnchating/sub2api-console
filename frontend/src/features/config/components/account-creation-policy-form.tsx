import { zodResolver } from "@hookform/resolvers/zod";
import { Save } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import { useForm } from "react-hook-form";

import type { AccountCreationPolicy } from "@/api";
import { FieldLabel } from "@/components/field-help-tooltip";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { SettingsFooter } from "./settings-footer";
import {
  accountCreationSettingsSchema,
  parseAccountModels,
  parseRetryStatusCodes,
  type AccountCreationSettingsValues,
} from "../lib/account-creation-settings-schema";

function accountCreationPolicyFormValues(
  policy: AccountCreationPolicy,
): AccountCreationSettingsValues {
  return {
    models: policy.models.join("\n"),
    concurrency: String(policy.concurrency),
    loadFactor: policy.load_factor ?? "",
    priority: String(policy.priority),
    poolMode: policy.pool_mode,
    retryCount: String(policy.pool_mode_retry_count),
    retryStatusCodes: policy.pool_mode_retry_status_codes.join(", "),
  };
}

function policyFromForm(values: AccountCreationSettingsValues): AccountCreationPolicy {
  return {
    models: parseAccountModels(values.models),
    concurrency: Number(values.concurrency),
    load_factor: values.loadFactor || null,
    priority: Number(values.priority),
    pool_mode: values.poolMode,
    pool_mode_retry_count: Number(values.retryCount),
    pool_mode_retry_status_codes: parseRetryStatusCodes(values.retryStatusCodes),
  };
}

export function AccountCreationPolicyForm(props: {
  formId: string;
  scopeLabel: string;
  policy: AccountCreationPolicy;
  pending: boolean;
  disabled?: boolean;
  forceDirty?: boolean;
  fillHeight?: boolean;
  submitLabel: string;
  onSubmit: (policy: AccountCreationPolicy) => void;
}) {
  const form = useForm<AccountCreationSettingsValues>({
    resolver: zodResolver(accountCreationSettingsSchema),
    defaultValues: accountCreationPolicyFormValues(props.policy),
  });
  const poolMode = form.watch("poolMode");

  useEffect(() => {
    form.reset(accountCreationPolicyFormValues(props.policy));
  }, [form, props.policy]);

  function accessibleName(field: string): string {
    return `${props.scopeLabel} ${field}`;
  }

  return (
    <form
      className={cn("flex min-w-0 flex-col", props.fillHeight && "h-full min-h-0 overflow-hidden")}
      data-testid="account-creation-policy-layout"
      onSubmit={form.handleSubmit((values) => props.onSubmit(policyFromForm(values)))}
    >
      <div
        data-slot={props.fillHeight ? "settings-scroll" : undefined}
        className={cn(
          "grid content-start items-start gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]",
          props.fillHeight && "min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3",
        )}
      >
        <div className="grid min-w-0 gap-4">
          <SettingsField
            label="账号模型"
            description="留空时自动同步上游全部模型；填写后仅添加指定模型。"
            error={form.formState.errors.models?.message}
          >
            <Textarea
              aria-label={accessibleName("账号模型")}
              aria-invalid={Boolean(form.formState.errors.models)}
              className="field-sizing-fixed h-44 max-h-44 resize-none"
              rows={5}
              disabled={props.disabled}
              placeholder="每行一个模型"
              {...form.register("models")}
            />
          </SettingsField>
        </div>

        <div className="grid min-w-0 content-start gap-4">
          <div className="grid gap-3 sm:grid-cols-3" data-testid="account-creation-routing-grid">
            <SettingsField label="并发上限" error={form.formState.errors.concurrency?.message}>
              <Input
                type="number"
                aria-label={accessibleName("并发上限")}
                aria-invalid={Boolean(form.formState.errors.concurrency)}
                min={1}
                max={10_000_000}
                disabled={props.disabled}
                {...form.register("concurrency")}
              />
            </SettingsField>
            <SettingsField
              label="负载因子"
              description="留空时跟随并发"
              error={form.formState.errors.loadFactor?.message}
            >
              <Input
                type="number"
                aria-label={accessibleName("负载因子")}
                aria-invalid={Boolean(form.formState.errors.loadFactor)}
                min={1}
                step="any"
                disabled={props.disabled}
                {...form.register("loadFactor")}
              />
            </SettingsField>
            <SettingsField
              label="优先级"
              description="数值越小越优先"
              error={form.formState.errors.priority?.message}
            >
              <Input
                type="number"
                aria-label={accessibleName("优先级")}
                aria-invalid={Boolean(form.formState.errors.priority)}
                min={1}
                max={10_000_000}
                disabled={props.disabled}
                {...form.register("priority")}
              />
            </SettingsField>
          </div>

          <div className="border-border overflow-hidden rounded-md border">
            <div className="flex items-center justify-between gap-4 px-3 py-2.5">
              <FieldLabel
                label="池模式"
                description="为新账号启用请求失败重试"
                htmlFor={`${props.formId}-pool-mode`}
              />
              <Switch
                id={`${props.formId}-pool-mode`}
                checked={poolMode}
                disabled={props.disabled}
                aria-label={accessibleName("开启池模式")}
                onCheckedChange={(checked) =>
                  form.setValue("poolMode", checked, { shouldDirty: true })
                }
              />
            </div>
            {poolMode ? (
              <div className="border-border grid gap-3 border-t px-3 py-3 sm:grid-cols-2">
                <SettingsField label="重试次数" error={form.formState.errors.retryCount?.message}>
                  <Input
                    type="number"
                    aria-label={accessibleName("重试次数")}
                    aria-invalid={Boolean(form.formState.errors.retryCount)}
                    min={0}
                    max={10}
                    disabled={props.disabled}
                    {...form.register("retryCount")}
                  />
                </SettingsField>
                <SettingsField
                  label="重试状态码"
                  description="使用逗号或空格分隔"
                  error={form.formState.errors.retryStatusCodes?.message}
                >
                  <Input
                    aria-label={accessibleName("重试状态码")}
                    aria-invalid={Boolean(form.formState.errors.retryStatusCodes)}
                    disabled={props.disabled}
                    placeholder="401, 403, 429"
                    {...form.register("retryStatusCodes")}
                  />
                </SettingsField>
              </div>
            ) : null}
          </div>
        </div>
      </div>
      <SettingsFooter className={props.fillHeight ? undefined : "mt-4 px-0 pb-0"}>
        <Button
          type="submit"
          className="h-auto min-h-8 max-w-full whitespace-normal break-words"
          disabled={
            props.pending || props.disabled || (!form.formState.isDirty && !props.forceDirty)
          }
        >
          <Save aria-hidden="true" />
          {props.pending ? "保存中…" : props.submitLabel}
        </Button>
      </SettingsFooter>
    </form>
  );
}

function SettingsField(props: {
  label: string;
  description?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="grid min-w-0 gap-1.5 text-sm">
      <FieldLabel label={props.label} description={!props.error ? props.description : undefined} />
      {props.children}
      {props.error ? <span className="text-destructive text-xs">{props.error}</span> : null}
    </div>
  );
}
