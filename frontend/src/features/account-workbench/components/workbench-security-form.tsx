import { useState, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import type {
  AccountStatus,
  WorkbenchSecurityInput,
  WorkbenchSecuritySource,
  WorkbenchOAuthCheckpoint,
} from "@/api";
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
import { securitySchema, type SecurityValues } from "../lib/security-schema";

export function WorkbenchSecurityForm(props: {
  accounts: AccountStatus[];
  source?: WorkbenchSecuritySource;
  checkpoint?: WorkbenchOAuthCheckpoint;
  disabled: boolean;
  onSubmit: (input: WorkbenchSecurityInput) => void;
}): ReactElement {
  const [confirm, setConfirm] = useState(false);
  const form = useForm<SecurityValues>({
    resolver: zodResolver(securitySchema),
    defaultValues: {
      account_id: "",
      source_oauth_id: props.source?.source_oauth_id,
      source_checkpoint_id: props.checkpoint?.id,
      operation: "totp",
      password: "",
      proxy_url: "",
    },
  });
  const accounts = props.accounts.filter(
    (value) => value.platform === "openai" && value.account_type === "oauth",
  );
  const selected = accounts.find((item) => item.id === form.watch("account_id"));
  const operation = form.watch("operation");
  const managed = !props.source && !props.checkpoint;
  const clear = (): void => setConfirm(false);
  return (
    <form
      aria-label="账号安全设置"
      className="grid min-w-0 gap-4"
      onSubmit={form.handleSubmit((value) => {
        if (props.disabled) return;
        if (!selected && managed) {
          form.setError("account_id", { message: "所选账号已不可用，请刷新列表后重选" });
          return;
        }
        if (!confirm) {
          setConfirm(true);
          return;
        }
        props.onSubmit({
          account_id: managed ? value.account_id : undefined,
          source_oauth_id: props.source?.source_oauth_id,
          source_checkpoint: props.checkpoint,
          scope: props.source?.scope,
          proxy_url: managed ? value.proxy_url || undefined : undefined,
          operation: value.operation,
          confirmed: true,
          password: value.operation === "password" ? value.password : undefined,
        });
        form.reset();
        setConfirm(false);
      })}
    >
      {managed ? (
        <FormField
          label="登录代理"
          htmlFor="security-proxy"
          error={form.formState.errors.proxy_url?.message}
        >
          <Input
            id="security-proxy"
            type="password"
            autoComplete="off"
            disabled={props.disabled}
            aria-invalid={!!form.formState.errors.proxy_url}
            {...form.register("proxy_url", { onChange: clear })}
          />
        </FormField>
      ) : null}
      <div className="grid min-w-0 gap-4 sm:grid-cols-2">
        {managed ? (
          <Controller
            name="account_id"
            control={form.control}
            render={({ field, fieldState }) => (
              <FormField
                label="安全设置账号"
                htmlFor="security-account"
                error={fieldState.error?.message}
              >
                <Select
                  value={field.value}
                  onValueChange={(value) => {
                    field.onChange(value);
                    clear();
                  }}
                  disabled={props.disabled}
                  itemToStringLabel={(id) =>
                    accounts.find((value) => value.id === id)?.name ?? "请选择账号"
                  }
                >
                  <SelectTrigger id="security-account" aria-invalid={!!fieldState.error}>
                    <SelectValue placeholder="请选择账号" />
                  </SelectTrigger>
                  <SelectContent>
                    {accounts.map((value) => (
                      <SelectItem key={value.id} value={value.id}>
                        {value.name}（ID {value.id}）
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormField>
            )}
          />
        ) : null}
        <Controller
          name="operation"
          control={form.control}
          render={({ field }) => (
            <FormField label="安全操作" htmlFor="security-operation">
              <Select
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value);
                  form.setValue("password", "");
                  clear();
                }}
                disabled={props.disabled}
                itemToStringLabel={(value) =>
                  value === "password" ? "设置密码" : "启用 TOTP 双重验证"
                }
              >
                <SelectTrigger id="security-operation">
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
      </div>
      {operation === "password" ? (
        <FormField
          label="新密码"
          htmlFor="security-password"
          error={form.formState.errors.password?.message}
          description="12～128 个字符，包含大小写字母、数字和符号。"
        >
          <Input
            id="security-password"
            type="password"
            autoComplete="new-password"
            disabled={props.disabled}
            aria-invalid={!!form.formState.errors.password}
            {...form.register("password", { onChange: clear })}
          />
        </FormField>
      ) : null}
      {!accounts.length && managed ? (
        <p className="text-sm text-muted-foreground">暂无可设置安全信息的 OpenAI OAuth 账号</p>
      ) : null}
      {confirm && (selected || !managed) ? (
        <section aria-label="确认安全操作" className="grid gap-2 rounded-md border p-4 text-sm">
          {props.source ? (
            <div className="grid min-w-0 gap-1 wrap-anywhere">
              <p>账号：{props.source.email}</p>
              <p>官方用户 ID：{props.source.user_id}</p>
              <p>工作区 ID：{props.source.workspace_id}</p>
            </div>
          ) : null}
          {selected ? (
            <p className="wrap-anywhere">
              账号：{selected.name}（ID {selected.id}）
            </p>
          ) : null}
          {props.checkpoint ? (
            <div className="grid min-w-0 gap-1 wrap-anywhere">
              <p>来源任务：{props.checkpoint.source_task_id}</p>
              <p>授权检查点：{props.checkpoint.id}</p>
              <p>此检查点将被使用，原授权不能再恢复。核对官方身份后才会执行安全操作。</p>
            </div>
          ) : null}
          <p>操作：{operation === "password" ? "设置密码" : "启用 TOTP 双重验证"}</p>
          <p className="text-muted-foreground">
            将打开官方登录页面核对账号。新凭据保存在服务器私有目录，请在操作完成后妥善备份。
          </p>
        </section>
      ) : null}
      <div className="flex flex-wrap justify-end gap-2">
        {confirm ? (
          <Button type="button" variant="outline" disabled={props.disabled} onClick={clear}>
            返回修改
          </Button>
        ) : null}
        <Button type="submit" disabled={props.disabled || (!accounts.length && managed)}>
          {confirm ? "确认并开始安全设置" : "查看操作范围"}
        </Button>
      </div>
    </form>
  );
}
