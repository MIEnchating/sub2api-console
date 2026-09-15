import { useState, type ReactElement } from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import type {
  AccountStatus,
  WorkbenchSecurityBatchInput,
  WorkbenchSecurityBatchSource,
  WorkbenchScope,
} from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { securityBatchSchema, type SecurityBatchValues } from "../lib/security-schema";

export function WorkbenchSecurityBatchForm(props: {
  accounts: AccountStatus[];
  sources?: Array<{ source: WorkbenchSecurityBatchSource; label: string }>;
  scope?: WorkbenchScope;
  disabled: boolean;
  onSubmit: (value: WorkbenchSecurityBatchInput) => void;
}): ReactElement {
  // Keep selected positions bound to the source IDs and versions shown on entry.
  const [sources] = useState(props.sources);
  const form = useForm<SecurityBatchValues>({
    resolver: zodResolver(securityBatchSchema),
    defaultValues: {
      account_ids: [],
      source_indexes: [],
      operation: "totp",
      password: "",
      proxy_url: "",
    },
  });
  const accounts = props.accounts.filter(
    (value) => value.platform === "openai" && value.account_type === "oauth",
  );
  const operation = form.watch("operation");
  return (
    <form
      aria-label="批量安全设置范围"
      className="grid min-w-0 gap-4"
      onSubmit={form.handleSubmit((value) => {
        props.onSubmit({
          account_ids: sources ? undefined : value.account_ids,
          sources: sources
            ? value.source_indexes?.map((index) => sources[index].source)
            : undefined,
          scope: props.scope,
          proxy_url: value.proxy_url || undefined,
          operation: value.operation,
          password: value.operation === "password" ? value.password : undefined,
        });
        form.setValue("password", "");
        form.setValue("proxy_url", "");
      })}
    >
      {!sources ? (
        <FormField
          label="登录代理"
          htmlFor="security-batch-proxy"
          error={form.formState.errors.proxy_url?.message}
        >
          <Input
            id="security-batch-proxy"
            type="password"
            autoComplete="off"
            disabled={props.disabled}
            aria-invalid={!!form.formState.errors.proxy_url}
            {...form.register("proxy_url")}
          />
        </FormField>
      ) : null}
      {sources ? (
        <Controller
          name="source_indexes"
          control={form.control}
          render={({ field }) => (
            <FormField label="授权来源范围" error={form.formState.errors.account_ids?.message}>
              <fieldset
                disabled={props.disabled}
                aria-label="可设置安全信息的授权来源"
                className="grid max-h-64 min-w-0 gap-2 overflow-auto border p-3"
              >
                {sources.map((item, index) => (
                  <label key={index} className="flex min-w-0 items-start gap-2 text-sm">
                    <Checkbox
                      checked={field.value?.includes(index) ?? false}
                      onCheckedChange={(checked) =>
                        field.onChange(
                          checked
                            ? [...(field.value ?? []), index]
                            : field.value?.filter((selected) => selected !== index),
                        )
                      }
                    />
                    <span className="min-w-0 wrap-anywhere">{item.label}</span>
                  </label>
                ))}
              </fieldset>
            </FormField>
          )}
        />
      ) : (
        <Controller
          name="account_ids"
          control={form.control}
          render={({ field, fieldState }) => (
            <FormField label="安全设置范围" error={fieldState.error?.message}>
              <label className="flex items-center gap-2 py-2 text-sm">
                <Checkbox
                  disabled={props.disabled || accounts.length === 0}
                  checked={
                    accounts.length > 0 && accounts.every((item) => field.value.includes(item.id))
                  }
                  onCheckedChange={(checked) =>
                    field.onChange(checked ? accounts.map((item) => item.id) : [])
                  }
                />
                选择全部账号（已选 {field.value.length} 个）
              </label>
              <fieldset
                disabled={props.disabled}
                aria-label="可设置安全信息的账号"
                aria-invalid={!!fieldState.error}
                className="grid max-h-64 min-w-0 gap-2 overflow-y-auto rounded-md border p-3 sm:grid-cols-2"
              >
                {accounts.map((account) => (
                  <label key={account.id} className="flex min-w-0 items-start gap-2 text-sm">
                    <Checkbox
                      checked={field.value.includes(account.id)}
                      onCheckedChange={(checked) =>
                        field.onChange(
                          checked
                            ? [...field.value, account.id]
                            : field.value.filter((id) => id !== account.id),
                        )
                      }
                    />
                    <span className="min-w-0 wrap-anywhere">
                      {account.name}（ID {account.id}）
                    </span>
                  </label>
                ))}
                {!accounts.length ? (
                  <p className="text-sm text-muted-foreground">暂无可设置安全信息的账号</p>
                ) : null}
              </fieldset>
            </FormField>
          )}
        />
      )}
      <Controller
        name="operation"
        control={form.control}
        render={({ field }) => (
          <FormField label="批量安全操作" htmlFor="security-batch-operation">
            <Select
              value={field.value}
              disabled={props.disabled}
              onValueChange={(value) => {
                field.onChange(value);
                form.setValue("password", "");
              }}
              itemToStringLabel={(value) =>
                value === "password" ? "设置密码" : "启用 TOTP 双重验证"
              }
            >
              <SelectTrigger id="security-batch-operation">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="totp">启用 TOTP 双重验证</SelectItem>
                <SelectItem value="password">设置密码</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
        )}
      />
      {operation === "password" ? (
        <FormField
          label="本批新密码"
          htmlFor="security-batch-password"
          error={form.formState.errors.password?.message}
          description="该密码将用于本批所选账号；12～128 个字符，包含大小写字母、数字和符号。"
        >
          <Input
            id="security-batch-password"
            type="password"
            autoComplete="new-password"
            disabled={props.disabled}
            {...form.register("password")}
          />
        </FormField>
      ) : null}
      <div className="flex justify-end">
        <Button
          type="submit"
          disabled={props.disabled || (sources ? sources.length === 0 : accounts.length === 0)}
        >
          预览批量安全操作
        </Button>
      </div>
    </form>
  );
}
