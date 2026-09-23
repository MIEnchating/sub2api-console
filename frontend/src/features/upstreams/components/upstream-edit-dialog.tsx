import { useUpstreamAllocationDraft } from "@/features/policy/hooks/use-upstream-allocation-draft";
import { UpstreamAllocationSetting } from "@/features/policy/components/upstream-allocation-setting";
import { ContentRetry } from "@/components/content-retry";
import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { FieldError } from "@/components/field-error";
import { ContentLoading } from "@/components/content-loading";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, LoaderCircle, RefreshCw, Save, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";

import { api, type UpstreamConfiguration, type UpstreamConfigurationUpdate } from "@/api";
import { Badge } from "@/components/ui/badge";
import { FieldLabel } from "@/components/field-help-tooltip";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
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
import { Switch } from "@/components/ui/switch";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { TaskCancelButton } from "@/components/task-startup-state";
import { AccountDeleteDialog } from "@/features/accounts/components/account-delete-dialog";
import { applyAccountDeletionProgress } from "@/features/accounts/lib/account-deletion-progress";
import {
  authModesForPlatform,
  parseStringMap,
  upstreamConnectionPayload,
  upstreamEditSchema,
  type UpstreamEditValues,
} from "../lib/upstream-edit-schema";
import { upstreamRateLabels } from "../lib/upstream-rate-labels";
import { StaleBindingCleanup } from "./stale-binding-cleanup";
import { UpstreamConcurrencyDetails } from "./upstream-concurrency";
import { notifyOperationError, operationErrorMessage } from "@/lib/operation-feedback";
import { notifyTaskResult } from "@/lib/task-result-feedback";
import { notifyProbeTaskResult } from "@/lib/probe-task-feedback";
import { sensitiveFieldPlaceholder } from "@/lib/sensitive-field";
import { configurableUpstreamTypeOptions } from "@/lib/domain-dictionaries";
import { terminalRefreshKeys } from "@/lib/task-refresh";
import { cn } from "@/lib/utils";
import { taskIsPending, taskPollInterval, taskStopsPolling } from "@/lib/task-state";
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
  content:
    "grid w-[min(56rem,calc(100vw-2rem))] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
  scrollArea: "overflow-x-clip",
  form: "grid min-w-0 max-w-full gap-4 overflow-x-clip",
} as const;

export const upstreamEditSectionOrder = [
  "concurrency",
  "connection",
  "authentication",
  "recharge",
  "accounts",
] as const;

const upstreamEditConnectionLabels = {
  upstreamAddress: "上游地址",
  accountBaseURL: "账号 Base URL",
} as const;

const upstreamEditFormID = "upstream-edit-form";

const emptyValues: UpstreamEditValues = {
  name: "",
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

function Field(props: {
  label: string;
  htmlFor?: string;
  description?: string;
  error?: string;
  children: ReactNode;
}) {
  let label: ReactNode = <span className="font-medium">{props.label}</span>;
  if (props.description) {
    label = (
      <FieldLabel label={props.label} htmlFor={props.htmlFor} description={props.description} />
    );
  } else if (props.htmlFor) {
    label = (
      <label className="font-medium" htmlFor={props.htmlFor}>
        {props.label}
      </label>
    );
  }
  return (
    <div className="grid min-w-0 gap-1.5 text-sm">
      {label}
      {props.children}
      {props.error ? (
        <FieldError
          id={props.htmlFor ? `${props.htmlFor}-error` : undefined}
          message={props.error}
        />
      ) : null}
    </div>
  );
}

function configured(value: boolean): string {
  return value ? "已配置，留空则不修改" : "未配置";
}

function displayNumber(value: string | null): string {
  return value === null || value === "" ? "未读取" : value;
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
  bindingId: number;
  upstreamId: string;
  upstreamKeyId: string;
  groupId: string;
  accountId: string;
  name: string | null;
  exists: boolean;
  host: string;
  group: string;
  groupStatus: string | null;
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
        bindingId: account.binding_id,
        upstreamId: group.upstream_id,
        upstreamKeyId: account.upstream_key_id,
        groupId: group.group_id ?? "",
        accountId: account.account_id,
        name: account.account_name,
        exists: account.account_exists,
        host: group.host,
        group: group.name,
        groupStatus: group.status,
        duplicate: (accountCounts.get(account.account_id) ?? 0) > 1,
      });
    }
  }

  return { bindings, accountCount: accountCounts.size };
}

export function currentUpstreamGroupStatus(status: string | null): {
  label: string;
  badgeVariant: "secondary" | "warning" | "destructive" | "outline";
} {
  const normalized = status?.trim().toLocaleLowerCase() ?? "";
  if (["missing", "deleted", "retired"].includes(normalized)) {
    return { label: "已不存在", badgeVariant: "destructive" };
  }
  if (normalized === "suspected") {
    return { label: "待确认", badgeVariant: "warning" };
  }
  if (["active", "enabled", "available", "ok", "1"].includes(normalized)) {
    return { label: "存在 · 启用", badgeVariant: "secondary" };
  }
  if (normalized) return { label: "存在 · 禁用", badgeVariant: "warning" };
  return { label: "状态未知", badgeVariant: "outline" };
}

function UpstreamAccountRowActions(props: {
  binding: CurrentUpstreamBinding;
  onChanged?: () => void;
}) {
  const queryClient = useQueryClient();
  const [activeAction, setActiveAction] = useState<string | null>(null);
  const [taskID, setTaskID] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const onChangedRef = useRef(props.onChanged);
  const handledTaskIDRef = useRef<string | null>(null);
  useEffect(() => {
    onChangedRef.current = props.onChanged;
  }, [props.onChanged]);
  const task = useQuery({
    queryKey: ["upstream-account-action", props.binding.host, props.binding.accountId, taskID],
    queryFn: () => api.task(taskID!),
    enabled: taskID !== null,
    refetchInterval: taskPollInterval,
  });
  const pending = taskIsPending(taskID, task);
  const actionPending = activeAction !== null;
  useEffect(() => {
    if (!taskStopsPolling(task.data) || activeAction === null || taskID === null) return;
    const completedTask = task.data;
    if (!completedTask) return;
    if (handledTaskIDRef.current === completedTask.id) return;
    handledTaskIDRef.current = completedTask.id;
    setTaskID(null);
    setActiveAction(null);
    const scope = activeAction === "探活测试" ? "active-probe" : "account-scheduling";
    const keys = terminalRefreshKeys(scope, completedTask);
    void Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
    applyAccountDeletionProgress(queryClient, completedTask);
    if (activeAction === "探活测试") {
      notifyProbeTaskResult(completedTask, props.binding.name ?? `账号 ${props.binding.accountId}`);
    } else {
      notifyTaskResult(completedTask, activeAction, {
        successMessage: `${props.binding.name ?? `账号 ${props.binding.accountId}`}：${activeAction}完成`,
      });
    }
    if (activeAction === "删除账号") setDeleteOpen(false);
    onChangedRef.current?.();
  }, [activeAction, props.binding.accountId, props.binding.name, queryClient, task.data, taskID]);

  async function startProbe(): Promise<void> {
    if (actionPending || !props.binding.exists) return;
    handledTaskIDRef.current = null;
    setActiveAction("探活测试");
    try {
      const queued = await api.runActiveProbe({ account_id: props.binding.accountId });
      setTaskID(queued.id);
    } catch (error) {
      setActiveAction(null);
      notifyOperationError(error, "探活测试启动失败");
    }
  }

  return (
    <>
      <div className="flex items-center justify-end gap-1">
        <TableActionButton
          label={activeAction === "探活测试" ? "正在探活" : "探活测试"}
          disabled={actionPending || !props.binding.exists}
          onClick={() => void startProbe()}
        >
          {activeAction === "探活测试" ? <LoaderCircle className="animate-spin" /> : <Activity />}
        </TableActionButton>
        {activeAction === "探活测试" && pending && taskID ? (
          <TaskCancelButton taskId={taskID} compact />
        ) : null}
        {props.binding.exists ? (
          <TableActionButton
            label="删除账号及上游 Key"
            tone="danger"
            disabled={actionPending}
            onClick={() => setDeleteOpen(true)}
          >
            <Trash2 />
          </TableActionButton>
        ) : (
          <StaleBindingCleanup
            host={props.binding.host}
            bindingId={props.binding.bindingId}
            upstreamId={props.binding.upstreamId}
            accountId={props.binding.accountId}
            upstreamKeyId={props.binding.upstreamKeyId}
            groupId={props.binding.groupId}
            groupName={props.binding.group}
            disabled={actionPending}
            onChanged={props.onChanged}
          />
        )}
      </div>
      <AccountDeleteDialog
        accountId={props.binding.accountId}
        open={deleteOpen}
        pending={actionPending}
        activeAction={activeAction}
        task={task.data}
        taskError={task.error}
        onOpenChange={(open) => {
          if (!actionPending) setDeleteOpen(open);
        }}
        onConfirm={(preview) => {
          handledTaskIDRef.current = null;
          setActiveAction("删除账号");
          void api.deleteAccount(preview).then(
            (queued) => setTaskID(queued.id),
            (error) => {
              setActiveAction(null);
              notifyOperationError(error, "删除账号启动失败");
            },
          );
        }}
      />
    </>
  );
}

export function UpstreamAccounts(props: {
  groups: UpstreamConfiguration["groups"];
  interactive?: boolean;
  onChanged?: () => void;
}) {
  const summary = currentUpstreamBindings(props.groups);
  return (
    <section className="grid min-w-0 gap-3" aria-labelledby="upstream-accounts">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 id="upstream-accounts" className="text-sm font-semibold">
            当前上游账号
          </h3>
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
          <div className="bg-muted/40 text-muted-foreground hidden min-w-0 px-3 py-2 text-xs font-medium sm:grid sm:grid-cols-[minmax(0,1.2fr)_minmax(8rem,1fr)_8rem_auto] sm:items-center sm:gap-3">
            <span>账号</span>
            <span>上游分组</span>
            <span>上游分组状态</span>
            <span className="text-right">操作</span>
          </div>
          {summary.bindings.map((binding) => {
            const groupStatus = currentUpstreamGroupStatus(binding.groupStatus);
            return (
              <div
                key={binding.key}
                className="grid min-w-0 gap-2 px-3 py-2.5 sm:grid-cols-[minmax(0,1.2fr)_minmax(8rem,1fr)_8rem_auto] sm:items-center sm:gap-3"
              >
                <div className="min-w-0">
                  <div className="flex min-w-0 items-center gap-2">
                    <strong className="block min-w-0 truncate text-sm font-medium">
                      {binding.name || `账号 ${binding.accountId}`}
                    </strong>
                    {binding.duplicate ? <Badge variant="warning">重复绑定</Badge> : null}
                  </div>
                  <span className="text-muted-foreground block truncate text-xs">
                    稳定账号 ID {binding.accountId}
                  </span>
                  {!binding.exists ? <Badge variant="destructive">账号不存在</Badge> : null}
                </div>
                <div className="min-w-0">
                  <span className="text-muted-foreground block truncate text-xs sm:hidden">
                    上游分组
                  </span>
                  <span className="block min-w-0 truncate text-sm">
                    {binding.group || "未记录"}
                  </span>
                </div>
                <div>
                  <span className="text-muted-foreground block text-xs sm:hidden">
                    上游分组状态
                  </span>
                  <Badge variant={groupStatus.badgeVariant}>{groupStatus.label}</Badge>
                </div>
                {props.interactive ? (
                  <UpstreamAccountRowActions binding={binding} onChanged={props.onChanged} />
                ) : null}
              </div>
            );
          })}
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
  const titleRef = useRef<HTMLHeadingElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const [tab, setTab] = useState<"configuration" | "accounts">("configuration");
  const orderedUpstreamTypes = useDictionaryOrder(
    "upstream_type",
    configurableUpstreamTypeOptions,
    (option) => option.value,
  );
  const queryClient = useQueryClient();
  const [showHeadersEditor, setShowHeadersEditor] = useState(false);
  const [clearHeaders, setClearHeaders] = useState(false);
  const configuration = useQuery({
    queryKey: ["upstream-configuration", props.host],
    queryFn: () => api.upstreamConfiguration(props.host!),
    enabled: props.host !== null,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
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
  const initializedHost = useRef<string | null>(null);
  const pendingSave = useRef<{ host: string; payload: UpstreamConfigurationUpdate } | null>(null);
  const isDirty = form.formState.isDirty;
  const platform = form.watch("upstream_type");
  const allocation = useUpstreamAllocationDraft(
    "upstreams",
    configuration.data?.upstream_id ?? "",
    props.host !== null &&
      Boolean(configuration.data?.upstream_id) &&
      configuration.data?.upstream_type.toLowerCase() === "sub2api" &&
      platform === "sub2api",
  );
  const authMode = form.watch("auth_mode");
  const authModes = useMemo(() => authModesForPlatform(platform), [platform]);

  useEffect(() => {
    setTab("configuration");
  }, [props.host]);

  useEffect(() => {
    if (props.host === null) {
      initializedHost.current = null;
      form.reset(emptyValues);
      return;
    }
    if (!configuration.data) return;
    if (initializedHost.current === props.host && isDirty) return;
    initializedHost.current = props.host;
    form.reset({
      ...emptyValues,
      name: configuration.data.name,
      base_url: configuration.data.base_url,
      account_base_url: configuration.data.account_base_url,
      upstream_type: configuration.data.upstream_type,
      auth_mode: configuration.data.auth_mode,
      entry: configuration.data.entry ?? "",
      recharge_rate: configuration.data.recharge_rate || "1",
    });
    setShowHeadersEditor(configuration.data.header_names.length > 0);
    setClearHeaders(false);
  }, [configuration.data, form, isDirty, props.host]);

  useEffect(() => {
    if (authModes.some((item) => item.value === authMode)) return;
    form.setValue("auth_mode", authModes[0]?.value ?? "custom_headers", {
      shouldValidate: true,
    });
  }, [authMode, authModes, form]);

  const save = useMutation({
    gcTime: 0,
    mutationFn: async (host: string) => {
      const submission = pendingSave.current;
      pendingSave.current = null;
      if (!submission || submission.host !== host)
        throw new Error("上游提交内容已清除，请重新填写");
      await allocation.save();
      return api.updateUpstreamConfiguration(host, submission.payload);
    },
    onSuccess: (value, host) => {
      const hostChanged = value.host !== host;
      queryClient.setQueryData(["upstream-configuration", host], value);
      if (hostChanged) {
        queryClient.removeQueries({ queryKey: ["upstream-configuration", host] });
        queryClient.setQueryData(["upstream-configuration", value.host], value);
      }
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
      void queryClient.invalidateQueries({ queryKey: ["upstream-groups"] });
      if (props.host !== host) {
        props.onSaved();
        return;
      }
      form.reset({
        ...emptyValues,
        name: value.name,
        base_url: value.base_url,
        account_base_url: value.account_base_url,
        upstream_type: value.upstream_type,
        auth_mode: value.auth_mode,
        entry: value.entry ?? "",
        recharge_rate: value.recharge_rate,
      });
      setShowHeadersEditor(value.header_names.length > 0);
      setClearHeaders(false);
      if (hostChanged) {
        toast.success("上游地址已更新，相关账号绑定已同步");
        props.onOpenChange(false);
      } else if (value.rate_sync_error || value.base_url_sync_error) {
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
    if (!props.host || save.isPending || pendingSave.current) return;
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
    if (clearHeaders) {
      payload.headers = {};
    } else if (values.headers.trim()) {
      try {
        payload.headers = parseStringMap(values.headers, "Headers");
      } catch (error) {
        form.setError("headers", {
          type: "manual",
          message: operationErrorMessage(error, "Headers 格式无效"),
        });
        return;
      }
    }
    if (values.auth_mode === "newapi_session" && values.cookies.trim()) {
      payload.cookies = { session: values.cookies.trim() };
    } else if (values.cookies.trim()) {
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
    pendingSave.current = { host: props.host, payload };
    save.mutate(props.host);
  }

  const data: UpstreamConfiguration | undefined = configuration.data;
  const presentation = data ? upstreamEditPresentation(data) : null;
  const supportsSharedConcurrency =
    data?.upstream_type.toLowerCase() === "sub2api" && platform === "sub2api";
  const showAccessToken = ["sub2api_user_token", "newapi_user_token", "bearer_token"].includes(
    authMode,
  );
  const showRefreshToken = authMode === "sub2api_user_token";
  const showAdminKey = authMode === "newapi_admin_key";
  const showNewApiSession = authMode === "newapi_session";
  const showUserId = showAdminKey || showNewApiSession;
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
    if (!showVaultLogin || !vaultConfiguration.data) return;
    if (selectedVaultEntry && vaultOptions.some((item) => item.entry === selectedVaultEntry))
      return;
    form.setValue("entry", defaultVaultEntryForHost(vaultOptions, props.host));
  }, [form, props.host, selectedVaultEntry, showVaultLogin, vaultConfiguration.data, vaultOptions]);

  return (
    <Dialog open={props.host !== null} onOpenChange={props.onOpenChange}>
      <DialogContent
        width="wide"
        height="tall"
        className={upstreamEditDialogLayout.content}
        initialFocus={titleRef}
      >
        <DialogHeader>
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <div className="min-w-0">
              <DialogTitle ref={titleRef} tabIndex={-1} className="outline-none">
                编辑上游
              </DialogTitle>
              <DialogDescription className="text-xs wrap-anywhere">{props.host}</DialogDescription>
            </div>
            <SegmentedControl role="tablist" aria-label="上游编辑分类" className="shrink-0">
              <SegmentedControlItem
                id="upstream-editor-tab-configuration"
                role="tab"
                aria-controls="upstream-editor-panel-configuration"
                selected={tab === "configuration"}
                onClick={() => {
                  setTab("configuration");
                  if (bodyRef.current) bodyRef.current.scrollTop = 0;
                }}
              >
                配置
              </SegmentedControlItem>
              <SegmentedControlItem
                id="upstream-editor-tab-accounts"
                role="tab"
                aria-controls="upstream-editor-panel-accounts"
                selected={tab === "accounts"}
                onClick={() => {
                  setTab("accounts");
                  if (bodyRef.current) bodyRef.current.scrollTop = 0;
                }}
              >
                关联账号
              </SegmentedControlItem>
            </SegmentedControl>
          </div>
        </DialogHeader>
        <DialogBody ref={bodyRef} className={upstreamEditDialogLayout.scrollArea}>
          {configuration.isLoading && <ContentLoading label="正在读取上游配置" />}
          {!configuration.isLoading && configuration.error && (
            <>
              <QueryErrorToast error={configuration.error} fallback="上游配置读取失败" />
              {!configuration.data && (
                <ContentRetry
                  onRetry={() => void configuration.refetch()}
                  pending={configuration.isFetching}
                />
              )}
            </>
          )}
          {configuration.data && (
            <form
              id={upstreamEditFormID}
              className={upstreamEditDialogLayout.form}
              onSubmit={form.handleSubmit(onSubmit)}
            >
              <div
                id="upstream-editor-panel-configuration"
                role="tabpanel"
                aria-labelledby="upstream-editor-tab-configuration"
                hidden={tab !== "configuration"}
                className={cn("min-w-0 gap-4", tab === "configuration" ? "grid" : "hidden")}
              >
                {supportsSharedConcurrency ? (
                  <div className="grid min-w-0 rounded-lg border bg-muted/10 lg:grid-cols-2">
                    <div className="flex min-w-0 items-center px-3 py-3">
                      <UpstreamConcurrencyDetails
                        limit={data?.concurrency_limit}
                        status={data?.concurrency_status}
                        allocated={data?.allocated_concurrency}
                        target={data?.target_concurrency}
                      />
                    </div>
                    {data?.upstream_id ? (
                      <div className="grid min-w-0 items-center border-t lg:border-t-0 lg:border-l">
                        <UpstreamAllocationSetting
                          key={data.upstream_id}
                          draft={allocation}
                          kind="upstreams"
                          id={data.upstream_id}
                          layout="row"
                          disabled={save.isPending}
                        />
                      </div>
                    ) : null}
                  </div>
                ) : null}
                <div
                  className="grid min-w-0 grid-cols-1 items-start gap-5 lg:grid-cols-2"
                  data-testid="upstream-editor-sections"
                >
                  <section
                    className="grid min-w-0 gap-3"
                    aria-labelledby="upstream-connection-title"
                  >
                    <div>
                      <h3 id="upstream-connection-title" className="text-sm font-semibold">
                        连接设置
                      </h3>
                    </div>
                    <div
                      className="grid min-w-0 gap-x-4 gap-y-3 sm:grid-cols-2"
                      data-testid="upstream-connection-fields"
                    >
                      <Field
                        label="名称"
                        htmlFor="upstream-edit-name"
                        error={form.formState.errors.name?.message}
                      >
                        <Input id="upstream-edit-name" {...form.register("name")} />
                      </Field>
                      <Field
                        label="平台"
                        htmlFor="upstream-edit-platform"
                        error={form.formState.errors.upstream_type?.message}
                      >
                        <Controller
                          control={form.control}
                          name="upstream_type"
                          render={({ field }) => (
                            <Select
                              value={field.value}
                              onValueChange={(value) => value && field.onChange(value)}
                            >
                              <SelectTrigger id="upstream-edit-platform">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {orderedUpstreamTypes.map((option) => (
                                  <SelectItem key={option.value} value={option.value}>
                                    {option.label}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          )}
                        />
                      </Field>
                      <Field
                        label={upstreamEditConnectionLabels.upstreamAddress}
                        htmlFor="upstream-edit-address"
                        error={form.formState.errors.base_url?.message}
                      >
                        <Input
                          id="upstream-edit-address"
                          {...form.register("base_url")}
                          placeholder="https://api.example.com"
                          aria-invalid={Boolean(form.formState.errors.base_url)}
                          aria-describedby={
                            form.formState.errors.base_url
                              ? "upstream-edit-address-error"
                              : undefined
                          }
                        />
                      </Field>
                      <Field
                        label={upstreamEditConnectionLabels.accountBaseURL}
                        htmlFor="upstream-edit-account-base-url"
                        error={form.formState.errors.account_base_url?.message}
                      >
                        <Input
                          id="upstream-edit-account-base-url"
                          {...form.register("account_base_url")}
                          placeholder="https://api.example.com"
                        />
                      </Field>
                    </div>
                  </section>
                  <section
                    className="grid min-w-0 gap-3 border-t pt-4 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-5"
                    aria-labelledby="upstream-auth-title"
                  >
                    <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
                      <h3 id="upstream-auth-title" className="text-sm font-semibold">
                        鉴权配置
                      </h3>
                      <p className="text-muted-foreground text-xs">凭据留空保留</p>
                    </div>
                    <div className="grid min-w-0 gap-x-4 gap-y-3 sm:grid-cols-2">
                      <div className="sm:col-span-2">
                        <Field
                          label="鉴权方式"
                          htmlFor="upstream-edit-auth-mode"
                          error={form.formState.errors.auth_mode?.message}
                        >
                          <Controller
                            control={form.control}
                            name="auth_mode"
                            render={({ field }) => (
                              <Select
                                value={field.value}
                                onValueChange={(value) => value && field.onChange(value)}
                              >
                                <SelectTrigger id="upstream-edit-auth-mode">
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
                      </div>
                      {showAccessToken ? (
                        <Field label="Token" htmlFor="upstream-edit-token">
                          <Input
                            id="upstream-edit-token"
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
                        <Field label="Refresh Token" htmlFor="upstream-edit-refresh-token">
                          <Input
                            id="upstream-edit-refresh-token"
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
                        <Field label="Admin Key" htmlFor="upstream-edit-admin-key">
                          <Input
                            id="upstream-edit-admin-key"
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
                        <Field label="User ID" htmlFor="upstream-edit-user-id">
                          <Input
                            id="upstream-edit-user-id"
                            placeholder={sensitiveFieldPlaceholder(
                              data?.has_user_id === true,
                              "输入 User ID",
                            )}
                            {...form.register("user_id")}
                          />
                        </Field>
                      ) : null}
                      {showVaultLogin ? (
                        <Field
                          label="密码箱密码项"
                          htmlFor="upstream-edit-vault-entry"
                          error={form.formState.errors.entry?.message}
                        >
                          <Select
                            value={form.watch("entry")}
                            onValueChange={(value) => {
                              if (!value) return;
                              form.setValue("entry", value, { shouldDirty: true });
                            }}
                          >
                            <SelectTrigger id="upstream-edit-vault-entry">
                              <SelectValue placeholder="选择密码箱项" />
                            </SelectTrigger>
                            <SelectContent>
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
                          <Field label="用户名" htmlFor="upstream-edit-username">
                            <Input
                              id="upstream-edit-username"
                              autoComplete="username"
                              {...form.register("username")}
                            />
                          </Field>
                          <Field label="密码" htmlFor="upstream-edit-password">
                            <Input
                              id="upstream-edit-password"
                              type="password"
                              autoComplete="current-password"
                              {...form.register("password")}
                            />
                          </Field>
                          <div className="flex min-w-0 items-center gap-2 sm:col-span-2">
                            <Switch
                              id="upstream-edit-save-to-vault"
                              checked={form.watch("save_to_vault")}
                              onCheckedChange={(checked) => form.setValue("save_to_vault", checked)}
                              aria-label="登录成功后保存到密码箱"
                            />
                            <label
                              className="min-w-0 cursor-pointer text-sm"
                              htmlFor="upstream-edit-save-to-vault"
                            >
                              登录成功后自动保存到密码箱
                            </label>
                          </div>
                          {form.watch("save_to_vault") ? (
                            <Field label="凭据名称（可选）" htmlFor="upstream-edit-entry">
                              <Input
                                id="upstream-edit-entry"
                                placeholder="默认使用 Host"
                                {...form.register("entry")}
                              />
                            </Field>
                          ) : null}
                        </>
                      ) : null}
                      {showNewApiSession ? (
                        <Field label="Session Cookie" htmlFor="upstream-edit-session-cookie">
                          <Input
                            id="upstream-edit-session-cookie"
                            type="password"
                            autoComplete="off"
                            placeholder={sensitiveFieldPlaceholder(
                              data?.cookie_names.includes("session") === true,
                              "输入 Session Cookie",
                            )}
                            {...form.register("cookies")}
                          />
                        </Field>
                      ) : null}
                      {headersAvailable ? (
                        <div className="grid min-w-0 gap-2 sm:col-span-2">
                          <div className="flex min-w-0 items-center justify-between gap-3">
                            <FieldLabel
                              label="自定义 Headers"
                              description={
                                data?.header_names.length
                                  ? `已配置：${data.header_names.join("、")}。留空保留，填写 JSON 替换全部 Header，关闭开关清空。`
                                  : "填写 JSON 添加自定义 Header。"
                              }
                              htmlFor="upstream-edit-custom-headers"
                              className="cursor-pointer text-sm"
                            />
                            <Switch
                              id="upstream-edit-custom-headers"
                              checked={showHeadersEditor}
                              onCheckedChange={(checked) => {
                                setShowHeadersEditor(checked);
                                setClearHeaders(!checked);
                                if (!checked) {
                                  form.setValue("headers", "", { shouldDirty: true });
                                }
                              }}
                              aria-label="添加自定义 Headers"
                            />
                          </div>
                          {showHeadersEditor ? (
                            <Field
                              label="Headers JSON"
                              htmlFor="upstream-edit-headers-json"
                              error={form.formState.errors.headers?.message}
                            >
                              <JsonEditorField
                                id="upstream-edit-headers-json"
                                aria-label="Headers JSON"
                                aria-invalid={Boolean(form.formState.errors.headers)}
                                placeholder={sensitiveFieldPlaceholder(
                                  Boolean(data?.header_names.length),
                                  '例如 {"Authorization":"Bearer ..."}',
                                )}
                                control={form.control}
                                name="headers"
                                disabled={save.isPending}
                              />
                            </Field>
                          ) : null}
                        </div>
                      ) : null}
                      {showCookies ? (
                        <Field
                          label="Cookies JSON"
                          htmlFor="upstream-edit-cookies-json"
                          error={form.formState.errors.cookies?.message}
                        >
                          <JsonEditorField
                            id="upstream-edit-cookies-json"
                            aria-label="Cookies JSON"
                            placeholder={sensitiveFieldPlaceholder(
                              Boolean(data?.cookie_names.length),
                              '例如 {"session":"..."}',
                            )}
                            control={form.control}
                            name="cookies"
                            disabled={save.isPending}
                          />
                        </Field>
                      ) : null}
                    </div>
                  </section>
                </div>

                <section
                  className="grid min-w-0 gap-3 border-t pt-4"
                  aria-labelledby="upstream-recharge-title"
                >
                  <h3 id="upstream-recharge-title" className="sr-only">
                    充值换算
                  </h3>
                  <div className="grid min-w-0 items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
                    <Field
                      label="充值比例"
                      description={upstreamRateLabels.mappingFormula}
                      htmlFor="upstream-edit-recharge-rate"
                      error={form.formState.errors.recharge_rate?.message}
                    >
                      <Input
                        id="upstream-edit-recharge-rate"
                        inputMode="decimal"
                        {...form.register("recharge_rate")}
                      />
                    </Field>
                    <div className="grid min-w-0 grid-cols-2 gap-3 text-sm">
                      <div className="grid min-w-0 gap-1 py-1">
                        <span className="text-muted-foreground text-xs">
                          {upstreamRateLabels.rawBalance}
                        </span>
                        <strong
                          aria-label={upstreamRateLabels.rawBalance}
                          className="min-w-0 font-medium break-all tabular-nums"
                        >
                          {presentation?.rawBalance ?? "未读取"}
                        </strong>
                      </div>
                      <div className="grid min-w-0 gap-1 py-1">
                        <span className="text-muted-foreground text-xs">
                          {upstreamRateLabels.mappedBalance}
                        </span>
                        <strong
                          aria-label={upstreamRateLabels.mappedBalance}
                          className="min-w-0 font-medium break-all tabular-nums"
                        >
                          {presentation?.mappedBalance ?? "未读取"}
                        </strong>
                      </div>
                    </div>
                  </div>
                </section>
              </div>

              <div
                id="upstream-editor-panel-accounts"
                role="tabpanel"
                aria-labelledby="upstream-editor-tab-accounts"
                hidden={tab !== "accounts"}
                className={cn("min-w-0", tab !== "accounts" && "hidden")}
              >
                <UpstreamAccounts
                  groups={data?.groups ?? []}
                  interactive
                  onChanged={() => void configuration.refetch()}
                />
              </div>
            </form>
          )}
        </DialogBody>
        <DialogFooter className="flex-row flex-wrap items-center justify-end">
          <Button type="button" variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          {tab === "configuration" ? (
            <Button
              type="submit"
              form={upstreamEditFormID}
              disabled={
                save.isPending ||
                !allocation.ready ||
                configuration.isLoading ||
                Boolean(configuration.error)
              }
            >
              {save.isPending ? <RefreshCw className="animate-spin" /> : <Save />}
              {save.isPending ? "保存中…" : "保存并重算"}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
