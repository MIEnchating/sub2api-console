import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, RefreshCw, Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import {
  api,
  ApiError,
  type AccountCreationPolicy,
  type AccountCreationSettings,
  type GroupStatus,
  type RuntimeConfig,
  type Task,
} from "@/api";
import { TaskProgressState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { cn } from "@/lib/utils";
import { AccountCreationPolicyForm } from "./account-creation-policy-form";
import { PlatformProbeModelsForm } from "./platform-probe-models-form";

const defaultScope = "default";

type SettingsView = "default" | "groups" | "probe";

type GroupOption = {
  id: string;
  name: string;
};

type SaveRequest = {
  settings: AccountCreationSettings;
  successMessage: string;
};

function fallbackSettings(concurrency: number, priority: number): AccountCreationSettings {
  return {
    default: {
      models: [],
      concurrency,
      load_factor: null,
      priority,
      pool_mode: false,
      pool_mode_retry_count: 3,
      pool_mode_retry_status_codes: [401, 403, 429],
    },
    groups: [],
    platform_probe_models: {},
  };
}

function groupOptions(
  groups: GroupStatus[] | undefined,
  settings: AccountCreationSettings,
): GroupOption[] {
  const names = new Map(
    (groups ?? []).flatMap((group) => (group.id ? [[group.id, group.name] as const] : [])),
  );
  for (const group of settings.groups) {
    if (!names.has(group.group_id)) names.set(group.group_id, `分组 ${group.group_id}`);
  }
  return [...names]
    .map(([id, name]) => ({ id, name }))
    .sort((left, right) => left.name.localeCompare(right.name, "zh-CN"));
}

function settingsErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 404) {
    return "当前后端未加载账号设置接口，请更新并重启后端服务";
  }
  return error instanceof Error ? error.message : "账号设置读取失败";
}

export function AccountCreationSettingsCard(props: {
  fallbackConcurrency: number;
  fallbackPriority: number;
  initialScope?: string;
}) {
  const queryClient = useQueryClient();
  const placeholder = useMemo(
    () => fallbackSettings(props.fallbackConcurrency, props.fallbackPriority),
    [props.fallbackConcurrency, props.fallbackPriority],
  );
  const settings = useQuery({
    queryKey: ["account-creation-settings"],
    queryFn: api.accountCreationSettings,
    placeholderData: placeholder,
  });
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const initialView: SettingsView =
    props.initialScope && props.initialScope !== defaultScope ? "groups" : "default";
  const [view, setView] = useState<SettingsView>(initialView);
  const [poolModeDialogOpen, setPoolModeDialogOpen] = useState(false);
  const [poolModeTaskID, setPoolModeTaskID] = useState<string | null>(null);
  const currentSettings = settings.data ?? placeholder;
  const availableGroups = useMemo(
    () => groupOptions(groups.data, currentSettings),
    [currentSettings, groups.data],
  );
  const poolModeTask = useQuery({
    queryKey: ["account-pool-mode-sync", poolModeTaskID],
    queryFn: () => api.task(poolModeTaskID!),
    enabled: Boolean(poolModeTaskID),
    refetchInterval: taskPollInterval,
  });

  const syncPoolMode = useMutation({
    mutationFn: () =>
      api.syncExistingAccountPoolMode((accounts.data ?? []).map((account) => account.id)),
    onSuccess: (task) => {
      setPoolModeDialogOpen(false);
      setPoolModeTaskID(task.id);
      queryClient.setQueryData(["account-pool-mode-sync", task.id], task);
    },
    onError: (error) => notifyOperationError(error, "已有账号池模式同步启动失败"),
  });

  useEffect(() => {
    const task = poolModeTask.data;
    if (!poolModeTaskID || !task || !taskStopsPolling(task)) return;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["accounts"] }),
      queryClient.invalidateQueries({ queryKey: ["logs"] }),
    ]);
    if (task.status === "succeeded") toast.success(poolModeTaskMessage(task));
    else if (task.status === "partial") toast.warning(poolModeTaskMessage(task));
    else if (task.status === "cancelled") toast.info(task.message || "已有账号池模式同步已取消");
    else toast.error(task.message || "已有账号池模式同步失败");
    setPoolModeTaskID(null);
  }, [poolModeTask.data, poolModeTaskID, queryClient]);

  const save = useMutation({
    mutationFn: (request: SaveRequest) => api.updateAccountCreationSettings(request.settings),
    onSuccess: (value, request) => {
      queryClient.setQueryData(["account-creation-settings"], value);
      queryClient.setQueryData<RuntimeConfig>(["config"], (current) => {
        if (!current) return current;
        return {
          ...current,
          account_default_concurrency: value.default.concurrency,
          account_default_priority: value.default.priority,
        };
      });
      toast.success(request.successMessage);
    },
    onError: (error) => notifyOperationError(error, "账号设置保存失败"),
  });

  function saveDefault(policy: AccountCreationPolicy): void {
    save.mutate({
      settings: {
        default: policy,
        groups: currentSettings.groups.map((group) => ({ ...group })),
        platform_probe_models: { ...currentSettings.platform_probe_models },
      },
      successMessage: "全局默认账号设置已保存",
    });
  }

  function saveGroup(
    groupID: string,
    groupName: string,
    policy: AccountCreationPolicy | null,
  ): void {
    const nextGroups = currentSettings.groups
      .filter((group) => group.group_id !== groupID)
      .map((group) => ({ ...group }));
    if (policy) nextGroups.push({ group_id: groupID, ...policy });
    save.mutate({
      settings: {
        default: currentSettings.default,
        groups: nextGroups,
        platform_probe_models: { ...currentSettings.platform_probe_models },
      },
      successMessage: policy ? `${groupName} 的账号设置已保存` : `${groupName} 已恢复全局默认`,
    });
  }

  function savePlatformProbeModels(models: Record<string, string>): void {
    save.mutate({
      settings: {
        default: currentSettings.default,
        groups: currentSettings.groups.map((group) => ({ ...group })),
        platform_probe_models: models,
      },
      successMessage: "默认探活模型已保存",
    });
  }

  if (settings.isPlaceholderData || (settings.isLoading && settings.data === undefined)) {
    return (
      <Card size="sm" className="h-full" aria-label="正在读取账号设置">
        <CardHeader>
          <CardTitle>账号设置</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3">
          <Skeleton className="h-9" />
          <Skeleton className="h-40" />
        </CardContent>
      </Card>
    );
  }

  const saveDisabled = save.isPending || Boolean(settings.error);
  const configuredGroupCount = currentSettings.groups.length;
  let groupSettingsContent = (
    <div className="border-border divide-border overflow-hidden rounded-md border divide-y">
      {availableGroups.map((group) => {
        const policy = currentSettings.groups.find((item) => item.group_id === group.id);
        return (
          <GroupSettingsEditor
            key={group.id}
            group={group}
            policy={policy}
            defaultPolicy={currentSettings.default}
            initiallyExpanded={props.initialScope === group.id}
            pending={save.isPending}
            disabled={saveDisabled}
            onSave={(nextPolicy) => saveGroup(group.id, group.name, nextPolicy)}
          />
        );
      })}
    </div>
  );
  if (groups.isLoading && groups.data === undefined) {
    groupSettingsContent = (
      <div className="grid gap-2" aria-label="正在读取分组列表">
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
      </div>
    );
  } else if (availableGroups.length === 0) {
    groupSettingsContent = (
      <div className="border-border text-muted-foreground rounded-md border px-3 py-6 text-center text-sm">
        暂无可配置分组
      </div>
    );
  }

  return (
    <Card size="sm" className="h-full min-h-0 min-w-0" data-testid="account-creation-settings-card">
      <CardHeader className="shrink-0">
        <CardTitle>账号设置</CardTitle>
        <CardDescription>设置新账号参数，为不同分组保存各自配置</CardDescription>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col group-data-[size=sm]/card:p-0">
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          {settings.error ? (
            <p className="text-destructive text-sm" role="alert">
              {settingsErrorMessage(settings.error)}
            </p>
          ) : null}

          {poolModeTaskID && poolModeTask.data && !taskStopsPolling(poolModeTask.data) ? (
            <TaskProgressState
              taskId={poolModeTaskID}
              message={poolModeTask.data.message}
              progress={poolModeTask.data.progress}
            />
          ) : null}

          <div className="border-border/70 flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-3 py-3">
            <div className="min-w-0">
              <p className="text-sm font-medium">同步池模式到已有账号</p>
              <p className="text-muted-foreground mt-0.5 text-xs leading-5">
                使用已保存的全局与分组策略，只更新池模式相关字段。
              </p>
            </div>
            <Button
              type="button"
              variant="outline"
              className="w-full sm:w-auto"
              disabled={
                accounts.isLoading ||
                accounts.isError ||
                Boolean(settings.error) ||
                accounts.data?.length === 0 ||
                save.isPending ||
                syncPoolMode.isPending ||
                Boolean(poolModeTaskID)
              }
              onClick={() => setPoolModeDialogOpen(true)}
            >
              <RefreshCw aria-hidden="true" />
              同步池模式到已有账号
            </Button>
          </div>

          <SegmentedControl
            className="mx-3 my-3 grid w-auto shrink-0 grid-cols-3"
            role="tablist"
            aria-label="账号设置范围"
          >
            <SegmentedControlItem
              type="button"
              role="tab"
              id="account-settings-default-tab"
              aria-controls="account-settings-default-panel"
              selected={view === "default"}
              onClick={() => setView("default")}
            >
              全局默认
            </SegmentedControlItem>
            <SegmentedControlItem
              type="button"
              role="tab"
              id="account-settings-groups-tab"
              aria-controls="account-settings-groups-panel"
              selected={view === "groups"}
              onClick={() => setView("groups")}
            >
              分组独立配置
              {configuredGroupCount > 0 ? ` ${configuredGroupCount}` : null}
            </SegmentedControlItem>
            <SegmentedControlItem
              type="button"
              role="tab"
              id="account-settings-probe-tab"
              aria-controls="account-settings-probe-panel"
              selected={view === "probe"}
              onClick={() => setView("probe")}
            >
              探活模型
            </SegmentedControlItem>
          </SegmentedControl>

          {view === "default" ? (
            <div
              className="min-h-0 flex-1 overflow-hidden"
              role="tabpanel"
              id="account-settings-default-panel"
              aria-labelledby="account-settings-default-tab"
            >
              <AccountCreationPolicyForm
                fillHeight
                formId="account-settings-default"
                scopeLabel="全局默认"
                policy={currentSettings.default}
                pending={save.isPending}
                disabled={Boolean(settings.error)}
                submitLabel="保存全局默认"
                onSubmit={saveDefault}
              />
            </div>
          ) : null}
          {view === "groups" ? (
            <div
              className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden"
              role="tabpanel"
              id="account-settings-groups-panel"
              aria-labelledby="account-settings-groups-tab"
            >
              <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 px-3 text-sm">
                <span className="font-medium">分组独立配置</span>
                <span className="text-muted-foreground tabular-nums">
                  {configuredGroupCount} / {availableGroups.length} 已配置
                </span>
              </div>
              {groups.error ? (
                <p className="text-destructive text-sm" role="alert">
                  分组列表读取失败，当前仅显示已保存的分组设置
                </p>
              ) : null}
              <div
                data-slot="settings-scroll"
                className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-3"
              >
                {groupSettingsContent}
              </div>
            </div>
          ) : null}
          {view === "probe" ? (
            <div
              className="min-h-0 flex-1 overflow-hidden"
              role="tabpanel"
              id="account-settings-probe-panel"
              aria-labelledby="account-settings-probe-tab"
            >
              <PlatformProbeModelsForm
                models={currentSettings.platform_probe_models}
                pending={save.isPending}
                disabled={Boolean(settings.error)}
                onSubmit={savePlatformProbeModels}
              />
            </div>
          ) : null}
        </div>
      </CardContent>
      <Dialog
        open={poolModeDialogOpen}
        onOpenChange={(open) => {
          if (!syncPoolMode.isPending) setPoolModeDialogOpen(open);
        }}
      >
        <DialogContent width="medium" showCloseButton={!syncPoolMode.isPending}>
          <DialogHeader>
            <DialogTitle>同步已有账号池模式</DialogTitle>
            <DialogDescription>
              将把当前已保存的池模式策略同步到 {accounts.data?.length ?? 0} 个已有账号。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 text-sm">
            <div className="border-border/70 grid gap-2 rounded-md border px-3 py-3">
              <PoolModePolicySummary label="全局默认" policy={currentSettings.default} />
              <p className="text-muted-foreground text-xs">
                另有 {configuredGroupCount} 个分组使用独立账号设置；这些账号将使用对应分组的池模式。
              </p>
            </div>
            <p className="text-muted-foreground leading-5">
              不会修改并发、优先级、账号模型和调度状态。账号同时命中多个不同池模式策略时将跳过，并记录在任务明细中。
            </p>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={syncPoolMode.isPending}
              onClick={() => setPoolModeDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              type="button"
              disabled={syncPoolMode.isPending || Boolean(settings.error) || !accounts.data?.length}
              onClick={() => syncPoolMode.mutate()}
            >
              <RefreshCw className={syncPoolMode.isPending ? "animate-spin" : undefined} />
              {syncPoolMode.isPending
                ? "正在创建任务"
                : `确认同步 ${accounts.data?.length ?? 0} 个账号`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function PoolModePolicySummary(props: { label: string; policy: AccountCreationPolicy }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <span className="font-medium">{props.label}</span>
      <span className="text-muted-foreground tabular-nums">
        {props.policy.pool_mode ? "池模式开启" : "池模式关闭"} · 重试
        {props.policy.pool_mode_retry_count} 次 ·{" "}
        {props.policy.pool_mode_retry_status_codes.join("、")}
      </span>
    </div>
  );
}

function poolModeTaskMessage(task: Task): string {
  const succeeded = taskResultCount(task, "succeeded");
  const unchanged = taskResultCount(task, "unchanged");
  const skipped = taskResultCount(task, "skipped");
  const failed = taskResultCount(task, "failed");
  return `池模式同步完成：已更新 ${succeeded}，无需更新 ${unchanged}，跳过 ${skipped}，失败 ${failed}`;
}

function taskResultCount(task: Task, key: string): number {
  const value = task.result[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function GroupSettingsEditor(props: {
  group: GroupOption;
  policy: AccountCreationPolicy | undefined;
  defaultPolicy: AccountCreationPolicy;
  initiallyExpanded: boolean;
  pending: boolean;
  disabled: boolean;
  onSave: (policy: AccountCreationPolicy | null) => void;
}) {
  const [expanded, setExpanded] = useState(props.initiallyExpanded);
  const [enabled, setEnabled] = useState(props.policy !== undefined);
  const [enabledEdited, setEnabledEdited] = useState(false);

  useEffect(() => {
    setEnabled(props.policy !== undefined);
    setEnabledEdited(false);
  }, [props.policy]);

  const summary = groupPolicySummary(props.policy, enabled, enabledEdited);
  const panelID = `account-group-settings-${props.group.id}`;

  return (
    <div data-testid={`account-group-settings-${props.group.id}`}>
      <div className="flex min-w-0 items-center gap-3 px-3 py-3">
        <button
          type="button"
          data-press-animation="none"
          className="focus-visible:ring-ring min-w-0 flex-1 rounded-sm text-left outline-none focus-visible:ring-2"
          aria-label={`${expanded ? "收起" : "展开"} ${props.group.name} 设置`}
          aria-expanded={expanded}
          aria-controls={panelID}
          onClick={() => setExpanded((current) => !current)}
        >
          <span className="block truncate font-medium">{props.group.name}</span>
          <span className="text-muted-foreground block truncate text-xs">{summary}</span>
        </button>
        <Switch
          checked={enabled}
          disabled={props.disabled}
          aria-label={`${props.group.name} 使用独立设置`}
          onCheckedChange={(checked) => {
            setEnabled(checked);
            setEnabledEdited(true);
            setExpanded(true);
          }}
        />
        <ChevronDown
          className={cn(
            "text-muted-foreground size-4 transition-transform",
            expanded && "rotate-180",
          )}
          aria-hidden="true"
        />
      </div>

      {expanded ? (
        <div className="border-border bg-muted/15 border-t px-3 py-4" id={panelID}>
          {enabled ? (
            <AccountCreationPolicyForm
              formId={`account-settings-group-${props.group.id}`}
              scopeLabel={props.group.name}
              policy={props.policy ?? props.defaultPolicy}
              pending={props.pending}
              disabled={props.disabled}
              forceDirty={enabledEdited}
              submitLabel={`保存 ${props.group.name} 设置`}
              onSubmit={props.onSave}
            />
          ) : (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="text-muted-foreground text-sm">该分组将使用全局默认配置</span>
              <Button
                type="button"
                disabled={props.disabled || !enabledEdited}
                onClick={() => props.onSave(null)}
              >
                <Save aria-hidden="true" />
                {props.pending ? "保存中…" : "保存继承设置"}
              </Button>
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}

function groupPolicySummary(
  policy: AccountCreationPolicy | undefined,
  enabled: boolean,
  enabledEdited: boolean,
): string {
  if (!enabled) return enabledEdited ? "待保存：恢复继承全局默认" : "继承全局默认";
  if (!policy) return "待保存独立配置";
  const models = policy.models.length > 0 ? `${policy.models.length} 个模型` : "自动同步模型";
  const loadFactor = policy.load_factor ?? "跟随并发";
  const poolMode = policy.pool_mode ? "池模式开启" : "池模式关闭";
  return `${models} · 并发 ${policy.concurrency} · 负载 ${loadFactor} · 优先级 ${policy.priority} · ${poolMode}`;
}
