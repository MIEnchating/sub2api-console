import { notifyOperationError } from "@/lib/operation-feedback";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { useEffect } from "react";
import { ApiError, type KumaConfig } from "@/api";
import { Input } from "@/components/ui/input";
import { configSchema, type ConfigValues } from "../lib/schemas";
import { FormField } from "@/components/form-field";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";

export function ConfigForm(props: {
  config: KumaConfig;
  pending: boolean;
  error?: Error | null;
  onSubmit: (values: ConfigValues) => void;
}) {
  const form = useForm<ConfigValues>({
    resolver: zodResolver(configSchema),
    defaultValues: {
      base_url: props.config.base_url,
      api_key: "",
      username: props.config.username,
      password: "",
      otp: "",
      disable_management: false,
    },
  });
  const disableManagement = form.watch("disable_management");
  const baseURL = form
    .watch("base_url")
    .replace(/\/dashboard\/?$/, "")
    .replace(/\/$/, "");
  const addressChanged = baseURL !== props.config.base_url;
  const keyRequired = !props.config.api_key_configured || addressChanged;
  const passwordRequired =
    !disableManagement &&
    !!form.watch("username") &&
    (!props.config.management_configured ||
      addressChanged ||
      form.watch("username") !== props.config.username);
  useEffect(() => {
    if (!props.error) return;
    const fields: Record<string, keyof ConfigValues> = {
      kuma_key_required: "api_key",
      kuma_api_key_rejected: "api_key",
      kuma_invalid_url: "base_url",
      kuma_connection_failed: "base_url",
      kuma_credentials_required: "password",
      kuma_auth_failed: "password",
      kuma_otp_required: "otp",
    };
    const field = props.error instanceof ApiError ? fields[props.error.code] : undefined;
    if (field) form.setError(field, { message: props.error.message });
    else notifyOperationError(props.error, "保存失败，请重试");
  }, [form, props.error]);
  const onSubmit = (values: ConfigValues): void => {
    if (
      (!props.config.api_key_configured ||
        values.base_url.replace(/\/dashboard\/?$/, "").replace(/\/$/, "") !==
          props.config.base_url) &&
      !values.api_key
    ) {
      form.setError("api_key", { message: "首次接入或更换地址时请输入 API 密钥" });
      return;
    }
    if (
      !values.disable_management &&
      values.username &&
      !values.password &&
      (!props.config.management_configured || values.username !== props.config.username)
    ) {
      form.setError("password", { message: "请输入管理账号的密码" });
      return;
    }
    props.onSubmit(values);
  };
  return (
    <form id="kuma-config" className="grid min-w-0 gap-3" onSubmit={form.handleSubmit(onSubmit)}>
      <div className="grid min-w-0 gap-3 lg:grid-cols-2">
        <Card size="sm" role="region" aria-labelledby="kuma-metrics-title">
          <CardHeader>
            <CardTitle id="kuma-metrics-title">服务地址与指标</CardTitle>
            <CardDescription>配置 API 密钥后可读取监控状态和指标。</CardDescription>
          </CardHeader>
          <CardContent>
            <fieldset disabled={props.pending} className="grid min-w-0 gap-4">
              <FormField
                htmlFor="kuma-config-base_url"
                label="服务地址"
                error={form.formState.errors.base_url?.message}
              >
                <Input
                  id="kuma-config-base_url"
                  aria-describedby="kuma-base_url-help"
                  aria-required={true}
                  {...form.register("base_url")}
                  placeholder="https://status.example.com"
                  aria-invalid={!!form.formState.errors.base_url}
                />
                <p id="kuma-base_url-help" className="text-xs font-normal text-muted-foreground">
                  必填 · Uptime Kuma 服务地址，例如 https://status.example.com。
                </p>
              </FormField>
              <FormField
                htmlFor="kuma-config-api_key"
                label="API 密钥"
                error={form.formState.errors.api_key?.message}
              >
                <Input
                  type="password"
                  autoComplete="new-password"
                  id="kuma-config-api_key"
                  aria-describedby="kuma-api_key-help"
                  aria-required={keyRequired}
                  {...form.register("api_key")}
                  placeholder={props.config.api_key_configured ? "已配置，留空保留" : "uk…"}
                  aria-invalid={!!form.formState.errors.api_key}
                />
                <p id="kuma-api_key-help" className="text-xs font-normal text-muted-foreground">
                  首次接入或更换服务地址时必填；同一地址已配置时选填，留空保留。仅用于读取指标。
                </p>
              </FormField>
            </fieldset>
          </CardContent>
        </Card>
        <Card size="sm" role="region" aria-labelledby="kuma-management-title">
          <CardHeader>
            <CardTitle id="kuma-management-title">管理账号</CardTitle>
            <CardDescription>需要管理监控项时填写，凭据由后端保存，不会回显。</CardDescription>
          </CardHeader>
          <CardContent>
            <fieldset disabled={props.pending} className="grid min-w-0 gap-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField
                  htmlFor="kuma-config-username"
                  label="管理账号（可选）"
                  error={form.formState.errors.username?.message}
                >
                  <Input
                    id="kuma-config-username"
                    aria-describedby="kuma-username-help"
                    {...form.register("username")}
                    autoComplete="off"
                    disabled={disableManagement}
                    aria-invalid={!!form.formState.errors.username}
                  />
                  <p id="kuma-username-help" className="text-xs font-normal text-muted-foreground">
                    选填 · 仅查看指标时无需填写；新增、编辑或暂停监控项时需配置管理账号及密码。
                  </p>
                </FormField>
                <FormField
                  htmlFor="kuma-config-password"
                  label="管理密码"
                  error={form.formState.errors.password?.message}
                >
                  <Input
                    type="password"
                    autoComplete="new-password"
                    id="kuma-config-password"
                    aria-describedby="kuma-password-help"
                    aria-required={passwordRequired}
                    {...form.register("password")}
                    disabled={disableManagement}
                    placeholder={
                      props.config.management_configured ? "已配置，留空保留" : "填写账号后必填"
                    }
                    aria-invalid={!!form.formState.errors.password}
                  />
                  <p id="kuma-password-help" className="text-xs font-normal text-muted-foreground">
                    填写管理账号后，首次配置、更换地址或账号时必填；原配置可留空保留。
                  </p>
                </FormField>
              </div>
              <FormField
                htmlFor="kuma-config-otp"
                label="两步验证码（已启用时填写）"
                error={form.formState.errors.otp?.message}
              >
                <Input
                  id="kuma-config-otp"
                  aria-describedby="kuma-otp-help"
                  {...form.register("otp")}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  disabled={disableManagement}
                  aria-invalid={!!form.formState.errors.otp}
                />
                <p id="kuma-otp-help" className="text-xs font-normal text-muted-foreground">
                  条件必填 · 使用账号密码登录且账号开启两步验证时，填写当前 6 位验证码；否则留空。
                </p>
              </FormField>
              {props.config.management_configured && (
                <label className="flex items-center gap-2 text-sm">
                  <Controller
                    control={form.control}
                    name="disable_management"
                    render={({ field }) => (
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={props.pending}
                      />
                    )}
                  />
                  移除管理凭据，仅查看指标
                </label>
              )}
              <p className="text-xs text-muted-foreground">
                登录失效时，请重新输入密码及当前验证码并保存。更换服务地址时需重新输入对应服务的凭据。
              </p>
            </fieldset>
          </CardContent>
        </Card>
      </div>
    </form>
  );
}
