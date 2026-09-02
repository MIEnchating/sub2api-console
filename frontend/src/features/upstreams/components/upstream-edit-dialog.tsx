import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";

import { api, type UpstreamConfiguration, type UpstreamConfigurationUpdate } from "@/api";
import { Badge } from "@/components/ui/badge";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  authModesForPlatform,
  parseStringMap,
  parseUpstreamBaseUrl,
  upstreamConnectionPayload,
  upstreamEditSchema,
  type UpstreamEditValues,
} from "../lib/upstream-edit-schema";
import { upstreamRateLabels } from "../lib/upstream-rate-labels";
import { notifyOperationError, operationErrorMessage } from "@/lib/operation-feedback";
import { sensitiveFieldPlaceholder } from "@/lib/sensitive-field";
import { configurableUpstreamTypeOptions } from "@/lib/domain-dictionaries";
import {
  defaultVaultEntryForHost,
  vaultEntriesForHost,
  vaultEntryLabel,
} from "@/lib/vault-entry-label";

type Props = {
  host: string | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
};

export const upstreamEditDialogLayout = {
  content: "grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
  scrollArea: "overflow-x-clip",
  form: "grid min-w-0 max-w-full gap-5",
} as const;

export const upstreamEditSectionOrder = ["connection", "recharge", "accounts"] as const;

export const upstreamEditConnectionLabels = {
  upstreamHost: "上游 Host",
  accountBaseURL: "账号 Base URL",
} as const;

const upstreamEditFormID = "upstream-edit-form";

const emptyValues: UpstreamEditValues = {
  name: "",
  base_url_protocol: "https",
  base_url: "",
  account_base_url: "",
  upstream_type: "sub2api",
  auth_mode: "sub2api_user_token",
  recharge_rate: "1",
  access_token: "",
  refresh_token: "",
  admin_key: "",
  user_id: "",
  headers: "",
  cookies: "",
  username: "",
  password: "",
  save_to_vault: false,
  entry: "",
};

function Field(props: { label: string; error?: string; children: ReactNode }) {
  return (
    <div className="grid min-w-0 gap-1.5 text-sm">
      <span className="font-medium">{props.label}</span>
      {props.children}
      {props.error ? <span className="text-destructive text-xs">{props.error}</span> : null}
    </div>
  );
}

function configured(value: boolean): string {
  return value ? "已配置，留空则不修改" : "未配置";
}

function displayNumber(value: string | null): string {
  return value === null || value === "" ? "未读取" : value;
}

function authModeLabel(value: string): string {
  return (
    authModesForPlatform("sub2api")
      .concat(authModesForPlatform("newapi"), authModesForPlatform("custom"))
      .find((item) => item.value === value)?.label ?? value
  );
}

export function upstreamEditPresentation(data: UpstreamConfiguration) {
  return {
    accessTokenState: configured(data.has_access_token),
    refreshTokenState: configured(data.has_refresh_token),
    adminKeyState: configured(data.has_admin_key),
    userIdState: configured(data.has_user_id),
    rawBalance: displayNumber(data.raw_balance),
    mappedBalance: displayNumber(data.balance),
  };
}

type CurrentUpstreamBinding = {
  key: string;
  accountId: string;
  name: string | null;
  exists: boolean;
  group: string;
  duplicate: boolean;
};

function currentUpstreamBindings(groups: UpstreamConfiguration["groups"]): {
  bindings: CurrentUpstreamBinding[];
  accountCount: number;
} {
  const accountCounts = new Map<string, number>();
  for (const group of groups) {
    for (const account of group.bound_accounts) {
      accountCounts.set(account.account_id, (accountCounts.get(account.account_id) ?? 0) + 1);
    }
  }

  const bindings: CurrentUpstreamBinding[] = [];
  for (const group of groups) {
    for (const account of group.bound_accounts) {
      bindings.push({
        key: `${group.group_id}:${account.binding_id}:${account.account_id}`,
        accountId: account.account_id,
        name: account.account_name,
        exists: account.account_exists,
        group: group.name,
        duplicate: (accountCounts.get(account.account_id) ?? 0) > 1,
      });
    }
  }

  return { bindings, accountCount: accountCounts.size };
}

export function UpstreamAccounts(props: { groups: UpstreamConfiguration["groups"] }) {
  const summary = currentUpstreamBindings(props.groups);
  return (
    <section className="grid min-w-0 gap-3 border-t pt-4" aria-labelledby="upstream-accounts">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 id="upstream-accounts" className="text-sm font-semibold">
            当前上游账号
          </h3>
          <p className="text-muted-foreground mt-1 text-xs">
            逐条显示当前上游所有分组中的账号绑定，重复账号会保留并标记。
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Badge variant="secondary">{summary.bindings.length} 条绑定</Badge>
          <Badge variant="outline">{summary.accountCount} 个账号</Badge>
        </div>
      </div>
      {summary.bindings.length ? (
        <div
          className="divide-border/70 max-h-72 divide-y overflow-x-hidden overflow-y-auto rounded-lg border"
          aria-label="当前上游全部账号"
        >
          {summary.bindings.map((binding) => (
            <div
              key={binding.key}
              className="grid min-w-0 gap-2 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
            >
              <div className="min-w-0">
                <strong className="block truncate text-sm font-medium">
                  {binding.name || `账号 ${binding.accountId}`}
                </strong>
                <span className="text-muted-foreground block truncate text-xs">
                  稳定账号 ID {binding.accountId}
                </span>
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">
                <span className="text-muted-foreground max-w-72 truncate text-xs">
                  上游分组 {binding.group || "未记录"}
                </span>
                {binding.duplicate ? <Badge variant="warning">重复绑定</Badge> : null}
                {!binding.exists ? <Badge variant="destructive">账号不存在</Badge> : null}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-muted-foreground rounded-lg border px-3 py-8 text-center text-sm">
          当前上游暂无绑定账号
        </div>
      )}
    </section>
  );
}

export function UpstreamEditDialog(props: Props) {
  const queryClient = useQueryClient();
  const [showHeadersEditor, setShowHeadersEditor] = useState(false);
  const configuration = useQuery({
    queryKey: ["upstream-configuration", props.host],
    queryFn: () => api.upstreamConfiguration(props.host!),
    enabled: props.host !== null,
    retry: false,
  });
  const vaultConfiguration = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: api.authRecoveryConfig,
    enabled: props.host !== null,
    staleTime: 15_000,
  });
  const form = useForm<UpstreamEditValues>({
    resolver: zodResolver(upstreamEditSchema),
    defaultValues: emptyValues,
  });
  const platform = form.watch("upstream_type");
  const authMode = form.watch("auth_mode");
  const authModes = useMemo(() => authModesForPlatform(platform), [platform]);

  useEffect(() => {
    if (!configuration.data) return;
    const parsedBaseUrl = parseUpstreamBaseUrl(configuration.data.base_url);
    form.reset({
      ...emptyValues,
      name: configuration.data.name,
      ...parsedBaseUrl,
      account_base_url: configuration.data.account_base_url,
      upstream_type: configuration.data.upstream_type,
      auth_mode: configuration.data.auth_mode,
      recharge_rate: configuration.data.recharge_rate || "1",
      headers: Object.keys(configuration.data.headers).length
        ? JSON.stringify(configuration.data.headers, null, 2)
        : "",
    });
    setShowHeadersEditor(configuration.data.header_names.length > 0);
  }, [configuration.data, form]);

  useEffect(() => {
    if (authModes.some((item) => item.value === authMode)) return;
    form.setValue("auth_mode", authModes[0]?.value ?? "custom_headers", {
      shouldValidate: true,
    });
  }, [authMode, authModes, form]);

  const save = useMutation({
    mutationFn: (payload: UpstreamConfigurationUpdate) =>
      api.updateUpstreamConfiguration(props.host!, payload),
    onSuccess: (value) => {
      const parsedBaseUrl = parseUpstreamBaseUrl(value.base_url);
      queryClient.setQueryData(["upstream-configuration", props.host], value);
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
      void queryClient.invalidateQueries({ queryKey: ["upstream-groups"] });
      form.reset({
        ...emptyValues,
        name: value.name,
        ...parsedBaseUrl,
        account_base_url: value.account_base_url,
        upstream_type: value.upstream_type,
        auth_mode: value.auth_mode,
        recharge_rate: value.recharge_rate,
        headers: Object.keys(value.headers).length ? JSON.stringify(value.headers, null, 2) : "",
      });
      setShowHeadersEditor(value.header_names.length > 0);
      if (value.rate_sync_error || value.base_url_sync_error) {
        toast.warning(
          `上游配置已保存，但账号同步排队失败：${value.rate_sync_error ?? value.base_url_sync_error}`,
        );
      } else if (value.rate_sync_task_id || value.base_url_sync_task_id) {
        toast.success("上游配置已保存，相关账号配置同步已排队");
      } else {
        toast.success("上游配置已保存");
      }
      props.onSaved();
    },
    onError: (error) => notifyOperationError(error, "上游配置保存失败"),
  });

  function onSubmit(values: UpstreamEditValues) {
    form.clearErrors(["entry", "headers", "cookies"]);
    const payload: UpstreamConfigurationUpdate = {
      name: values.name.trim(),
      ...upstreamConnectionPayload(values),
      upstream_type: values.upstream_type,
      auth_mode: values.auth_mode,
      recharge_rate: values.recharge_rate,
    };
    if (values.access_token.trim()) payload.access_token = values.access_token.trim();
    if (values.refresh_token.trim()) payload.refresh_token = values.refresh_token.trim();
    if (values.admin_key.trim()) payload.admin_key = values.admin_key.trim();
    if (values.user_id.trim()) payload.user_id = values.user_id.trim();
    if (form.getFieldState("headers").isDirty) {
      try {
        payload.headers = values.headers.trim() ? parseStringMap(values.headers, "Headers") : {};
      } catch (error) {
        form.setError("headers", {
          type: "manual",
          message: operationErrorMessage(error, "Headers 格式无效"),
        });
        return;
      }
    }
    if (values.cookies.trim()) {
      try {
        payload.cookies = parseStringMap(values.cookies, "Cookies");
      } catch (error) {
        form.setError("cookies", {
          type: "manual",
          message: operationErrorMessage(error, "Cookies 格式无效"),
        });
        return;
      }
    }
    if (values.username.trim()) payload.username = values.username.trim();
    if (values.password) payload.password = values.password;
    if (["sub2api_user_login", "newapi_user_login"].includes(values.auth_mode)) {
      if (!values.entry.trim()) {
        form.setError("entry", { type: "manual", message: "请选择一个密码箱项" });
        return;
      }
      payload.entry = values.entry.trim();
    }
    if (values.save_to_vault) {
      payload.save_to_vault = true;
      if (values.entry.trim()) payload.entry = values.entry.trim();
    }
    save.mutate(payload);
  }

  const data: UpstreamConfiguration | undefined = configuration.data;
  const presentation = data ? upstreamEditPresentation(data) : null;
  const showAccessToken = ["sub2api_user_token", "newapi_user_token", "bearer_token"].includes(
    authMode,
  );
  const showRefreshToken = authMode === "sub2api_user_token";
  const showAdminKey = authMode === "newapi_admin_key";
  const showUserId = authMode === "newapi_admin_key";
  const showManualLogin = ["sub2api_manual_login", "newapi_manual_login"].includes(authMode);
  const showVaultLogin = ["sub2api_user_login", "newapi_user_login"].includes(authMode);
  const headersAvailable =
    ["sub2api", "newapi", "oneapi"].includes(platform) || authMode === "custom_headers";
  const showCookies = authMode === "custom_headers" || authMode === "cookie";
  const vaultOptions = useMemo(
    () =>
      vaultEntriesForHost(vaultConfiguration.data?.vault_entries ?? [], props.host, {
        requireEmail: platform === "sub2api",
      }),
    [platform, props.host, vaultConfiguration.data?.vault_entries],
  );
  const selectedVaultEntry = form.watch("entry");
  useEffect(() => {
    if (!showVaultLogin) return;
    if (selectedVaultEntry && vaultOptions.some((item) => item.entry === selectedVaultEntry))
      return;
    form.setValue("entry", defaultVaultEntryForHost(vaultOptions, props.host));
  }, [form, props.host, selectedVaultEntry, showVaultLogin, vaultOptions]);

  return (
    <Dialog open={props.host !== null} onOpenChange={props.onOpenChange}>
      <DialogContent width="wide" height="tall" className={upstreamEditDialogLayout.content}>
        <DialogHeader>
          <DialogTitle>编辑上游</DialogTitle>
        </DialogHeader>
        <DialogBody className={upstreamEditDialogLayout.scrollArea}>
          {configuration.isLoading ? (
            <div className="grid gap-3" aria-label="正在读取上游配置">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-32 w-full" />
              <Skeleton className="h-44 w-full" />
            </div>
          ) : configuration.error ? (
            <QueryErrorToast error={configuration.error} fallback="上游配置读取失败" />
          ) : (
            <form
              id={upstreamEditFormID}
              className={upstreamEditDialogLayout.form}
              onSubmit={form.handleSubmit(onSubmit)}
            >
              <section className="grid min-w-0 gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-sm font-semibold">连接与鉴权</h3>
                  <Badge variant="outline">{authModeLabel(authMode)}</Badge>
                </div>
                <div className="grid min-w-0 gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                  <Field label="名称" error={form.formState.errors.name?.message}>
                    <Input {...form.register("name")} />
                  </Field>
                  <Field
                    label={upstreamEditConnectionLabels.upstreamHost}
                    error={form.formState.errors.base_url?.message}
                  >
                    <div className="flex min-w-0 gap-2">
                      <Controller
                        control={form.control}
                        name="base_url_protocol"
                        render={({ field }) => (
                          <Select value={field.value} onValueChange={field.onChange}>
                            <SelectTrigger className="w-[6.75rem] shrink-0">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="https">HTTPS</SelectItem>
                              <SelectItem value="http">HTTP</SelectItem>
                            </SelectContent>
                          </Select>
                        )}
                      />
                      <Input {...form.register("base_url")} placeholder="api.example.com" />
                    </div>
                  </Field>
                  <Field
                    label={upstreamEditConnectionLabels.accountBaseURL}
                    error={form.formState.errors.account_base_url?.message}
                  >
                    <Input
                      {...form.register("account_base_url")}
                      placeholder="https://api.example.com"
                    />
                  </Field>
                  <Field label="平台" error={form.formState.errors.upstream_type?.message}>
                    <Controller
                      control={form.control}
                      name="upstream_type"
                      render={({ field }) => (
                        <Select
                          value={field.value}
                          onValueChange={(value) => value && field.onChange(value)}
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {configurableUpstreamTypeOptions.map((option) => (
                              <SelectItem key={option.value} value={option.value}>
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </Field>
                  <Field label="鉴权方式" error={form.formState.errors.auth_mode?.message}>
                    <Controller
                      control={form.control}
                      name="auth_mode"
                      render={({ field }) => (
                        <Select
                          value={field.value}
                          onValueChange={(value) => value && field.onChange(value)}
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {authModes.map((item) => (
                              <SelectItem key={item.value} value={item.value}>
                                {item.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </Field>
                  {showAccessToken ? (
                    <Field label={`Token（${presentation?.accessTokenState ?? "未配置"}）`}>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        placeholder={sensitiveFieldPlaceholder(
                          data?.has_access_token === true,
                          "输入 Token",
                        )}
                        {...form.register("access_token")}
                      />
                    </Field>
                  ) : null}
                  {showRefreshToken ? (
                    <Field
                      label={`Refresh Token（${presentation?.refreshTokenState ?? "未配置"}）`}
                    >
                      <Input
                        type="password"
                        autoComplete="new-password"
                        placeholder={sensitiveFieldPlaceholder(
                          data?.has_refresh_token === true,
                          "输入 Refresh Token",
                        )}
                        {...form.register("refresh_token")}
                      />
                    </Field>
                  ) : null}
                  {showAdminKey ? (
                    <Field label={`Admin Key（${presentation?.adminKeyState ?? "未配置"}）`}>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        placeholder={sensitiveFieldPlaceholder(
                          data?.has_admin_key === true,
                          "输入 Admin Key",
                        )}
                        {...form.register("admin_key")}
                      />
                    </Field>
                  ) : null}
                  {showUserId ? (
                    <Field label={`User ID（${presentation?.userIdState ?? "未配置"}）`}>
                      <Input
                        placeholder={sensitiveFieldPlaceholder(
                          data?.has_user_id === true,
                          "输入 User ID",
                        )}
                        {...form.register("user_id")}
                      />
                    </Field>
                  ) : null}
                  {showVaultLogin ? (
                    <Field label="密码箱密码项" error={form.formState.errors.entry?.message}>
                      <Select
                        value={form.watch("entry")}
                        onValueChange={(value) => {
                          if (!value) return;
                          form.setValue("entry", value);
                        }}
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="选择密码箱项" />
                        </SelectTrigger>
                        <SelectContent className="min-w-[20rem]">
                          {vaultOptions.map((item) => (
                            <SelectItem key={item.entry} value={item.entry}>
                              {vaultEntryLabel(item)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                  ) : null}
                  {showManualLogin ? (
                    <>
                      <Field label="用户名">
                        <Input autoComplete="username" {...form.register("username")} />
                      </Field>
                      <Field label="密码">
                        <Input
                          type="password"
                          autoComplete="current-password"
                          {...form.register("password")}
                        />
                      </Field>
                      <div className="flex items-center gap-2 sm:col-span-2">
                        <Switch
                          id="upstream-edit-save-to-vault"
                          checked={form.watch("save_to_vault")}
                          onCheckedChange={(checked) => form.setValue("save_to_vault", checked)}
                          aria-label="登录成功后保存到密码箱"
                        />
                        <label
                          className="cursor-pointer text-sm"
                          htmlFor="upstream-edit-save-to-vault"
                        >
                          登录成功后自动保存到密码箱
                        </label>
                      </div>
                      {form.watch("save_to_vault") ? (
                        <Field label="凭据名称（可选）">
                          <Input placeholder="默认使用 Host" {...form.register("entry")} />
                        </Field>
                      ) : null}
                    </>
                  ) : null}
                  {headersAvailable ? (
                    <div className="grid min-w-0 gap-2 sm:col-span-2">
                      <div className="flex min-w-0 items-center justify-between gap-3">
                        <label
                          className="min-w-0 cursor-pointer"
                          htmlFor="upstream-edit-custom-headers"
                        >
                          <p className="text-sm font-medium">自定义 Headers</p>
                          <p className="text-muted-foreground text-xs">
                            {data?.header_names.length
                              ? `已配置：${data.header_names.join("、")}`
                              : "默认不添加"}
                          </p>
                        </label>
                        <Switch
                          id="upstream-edit-custom-headers"
                          checked={showHeadersEditor}
                          onCheckedChange={(checked) => {
                            setShowHeadersEditor(checked);
                            if (!checked) {
                              form.setValue("headers", "", { shouldDirty: true });
                            }
                          }}
                          aria-label="添加自定义 Headers"
                        />
                      </div>
                      {showHeadersEditor ? (
                        <Field label="Headers JSON" error={form.formState.errors.headers?.message}>
                          <Textarea
                            className="min-h-24"
                            placeholder={sensitiveFieldPlaceholder(
                              Boolean(data?.header_names.length),
                              '例如 {"Authorization":"Bearer ..."}',
                            )}
                            {...form.register("headers")}
                          />
                        </Field>
                      ) : null}
                    </div>
                  ) : null}
                  {showCookies ? (
                    <Field
                      label={`Cookies JSON（${data?.cookie_names.length ? data.cookie_names.join("、") : "未配置"}）`}
                      error={form.formState.errors.cookies?.message}
                    >
                      <Textarea
                        className="min-h-24"
                        placeholder={sensitiveFieldPlaceholder(
                          Boolean(data?.cookie_names.length),
                          '例如 {"session":"..."}',
                        )}
                        {...form.register("cookies")}
                      />
                    </Field>
                  ) : null}
                </div>
              </section>

              <section className="grid min-w-0 gap-3 border-t pt-4">
                <div>
                  <h3 className="text-sm font-semibold">充值换算</h3>
                  <p className="text-muted-foreground mt-1 text-xs">
                    {upstreamRateLabels.mappingFormula}
                  </p>
                </div>
                <div className="grid min-w-0 items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
                  <Field label="充值比例" error={form.formState.errors.recharge_rate?.message}>
                    <Input inputMode="decimal" {...form.register("recharge_rate")} />
                  </Field>
                  <div className="grid min-w-0 grid-cols-2 divide-x overflow-hidden rounded-lg border text-sm">
                    <div className="grid min-w-0 gap-1 px-3 py-2">
                      <span className="text-muted-foreground text-xs">
                        {upstreamRateLabels.rawBalance}
                      </span>
                      <strong className="font-medium">
                        {presentation?.rawBalance ?? "未读取"}
                      </strong>
                    </div>
                    <div className="grid min-w-0 gap-1 px-3 py-2">
                      <span className="text-muted-foreground text-xs">
                        {upstreamRateLabels.mappedBalance}
                      </span>
                      <strong className="font-medium">
                        {presentation?.mappedBalance ?? "未读取"}
                      </strong>
                    </div>
                  </div>
                </div>
              </section>

              <UpstreamAccounts groups={data?.groups ?? []} />
            </form>
          )}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button
            type="submit"
            form={upstreamEditFormID}
            disabled={save.isPending || configuration.isLoading || Boolean(configuration.error)}
          >
            {save.isPending ? <RefreshCw className="animate-spin" /> : <Save />}
            {save.isPending ? "保存中…" : "保存并重算"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
