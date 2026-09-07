import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCheck, CheckCircle2, CircleAlert, CircleSlash2, Search, XCircle } from "lucide-react";

import {
  api,
  ApiError,
  type AccountModelSelection,
  type AccountModelSyncAccount,
  type AccountModelSyncPreview,
  type Task,
} from "@/api";
import { FieldLabel } from "@/components/field-help-tooltip";
import { MultiSelect } from "@/components/multi-select";
import {
  TaskCancelButton,
  TaskProgressState,
  TaskStartupState,
} from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  operationDialogHeight,
  operationDialogWidth,
} from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { accountPlatformLabel } from "@/features/accounts/lib/account-labels";
import { operationErrorMessage } from "@/lib/operation-feedback";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { cn } from "@/lib/utils";

type ModelSyncTaskItem = {
  accountId: string;
  accountName: string;
  status: string;
  error: string;
};

type ModelSyncProbeItem = {
  accountId: string;
  groupName: string;
  result: string;
  error: string;
  requestModel: string;
};

type ModelSyncDialogStage = "pending" | "selection" | "result";

export function accountModelSyncDialogLayout(stage: ModelSyncDialogStage) {
  const expanded = stage === "result";
  return {
    width: operationDialogWidth(stage !== "pending", "table"),
    height: operationDialogHeight(expanded),
    content: expanded
      ? "h-[min(52rem,calc(100svh-2rem))] grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      : "grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
  } as const;
}

export function successfulModelSyncAccountIDs(task: Task | undefined): string[] {
  return modelSyncTaskItems(task)
    .filter((item) => item.status === "succeeded")
    .map((item) => item.accountId);
}

export function preferredAccountProbeModel(
  account: AccountModelSyncAccount,
  excludedModels: Set<string>,
): string {
  const available = account.models.filter((model) => !excludedModels.has(model));
  const configured = available.find(
    (model) => model.toLocaleLowerCase() === account.probe_model.toLocaleLowerCase(),
  );
  return configured ?? available[0] ?? "";
}

export function availableUnifiedProbeModels(selections: AccountModelSelection[]): string[] {
  const models = new Map<string, string>();
  for (const selection of selections) {
    for (const model of selection.models) {
      const key = model.toLocaleLowerCase();
      if (!models.has(key)) models.set(key, model);
    }
  }
  return Array.from(models.values()).sort((left, right) => left.localeCompare(right));
}

function isModelCatalogChangedError(error: unknown): boolean {
  return error instanceof ApiError && error.code === "model_catalog_changed";
}

type ScopeModelExclusions = ReadonlyMap<string, ReadonlySet<string>>;

function accountPlatformKey(
  account: AccountModelSyncAccount,
  accountPlatforms: ReadonlyMap<string, string | null>,
): string {
  const listedPlatform = accountPlatforms.get(account.account_id)?.trim();
  const platform = (listedPlatform || account.platform?.trim() || "").toLocaleLowerCase();
  return platform || "unclassified";
}

function modelSyncScopeKey(platform: string, group: string): string {
  return `${platform}\u0000${group.toLocaleLowerCase()}`;
}

export function selectedAccountModels(
  preview: AccountModelSyncPreview,
  accountPlatforms: ReadonlyMap<string, string | null>,
  accountGroups: ReadonlyMap<string, string[]>,
  exclusions: ScopeModelExclusions,
): AccountModelSelection[] {
  const blocked = new Set(preview.blocked_models.map((model) => model.toLocaleLowerCase()));
  return preview.accounts.map((account) => {
    const platform = accountPlatformKey(account, accountPlatforms);
    const scopes = uniqueAccountGroups(accountGroups.get(account.account_id)).map((group) =>
      modelSyncScopeKey(platform, group),
    );
    const models = account.models.filter((model) => {
      const modelKey = model.toLocaleLowerCase();
      if (blocked.has(modelKey)) return false;
      return scopes.some((scope) => !exclusions.get(scope)?.has(modelKey));
    });
    return { account_id: account.account_id, models };
  });
}

export function modelSyncTaskItems(task: Task | undefined): ModelSyncTaskItem[] {
  if (!task || !Array.isArray(task.result.items)) return [];
  return task.result.items.flatMap((raw) => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const item = raw as Record<string, unknown>;
    return [
      {
        accountId: String(item.account_id ?? ""),
        accountName: String(item.account_name ?? ""),
        status: String(item.status ?? "failed"),
        error: String(item.error ?? ""),
      },
    ];
  });
}

export function modelSyncProbeItems(task: Task | undefined): ModelSyncProbeItem[] {
  const probe = task?.result.probe;
  if (typeof probe !== "object" || probe === null || Array.isArray(probe)) return [];
  const results = (probe as Record<string, unknown>).results;
  if (!Array.isArray(results)) return [];
  return results.flatMap((raw) => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const item = raw as Record<string, unknown>;
    return [
      {
        accountId: String(item.account_id ?? ""),
        groupName: String(item.group_name ?? ""),
        result: String(item.result ?? "失败"),
        error: String(item.failure_reason ?? ""),
        requestModel: String(item.request_model ?? ""),
      },
    ];
  });
}

export function AccountModelSyncDialog(props: {
  open: boolean;
  accountIds: string[];
  accountPlatforms: ReadonlyMap<string, string | null>;
  accountGroups: ReadonlyMap<string, string[]>;
  onOpenChange: (open: boolean) => void;
  onCompleted: () => void;
}) {
  const queryClient = useQueryClient();
  const [discoveryTaskId, setDiscoveryTaskId] = useState<string | null>(null);
  const [applyTaskId, setApplyTaskId] = useState<string | null>(null);
  const [scopeExclusions, setScopeExclusions] = useState<Map<string, Set<string>>>(() => new Map());
  const [probeModels, setProbeModels] = useState<string[]>([]);
  const [catalogRefreshMessage, setCatalogRefreshMessage] = useState<string | null>(null);
  const startedTarget = useRef<string | null>(null);
  const initializedFingerprint = useRef<string | null>(null);
  const completedTask = useRef<string | null>(null);
  const targetKey = props.accountIds.join("\u0000");
  const discovery = useMutation({
    mutationFn: () => api.discoverAccountModels(props.accountIds),
    onSuccess: (task) => setDiscoveryTaskId(task.id),
  });
  const discoveryTask = useQuery({
    queryKey: ["account-model-discovery", discoveryTaskId],
    queryFn: () => api.task(discoveryTaskId!),
    enabled: Boolean(discoveryTaskId),
    refetchInterval: taskPollInterval,
  });
  const discoveredAccountIDs = useMemo(
    () => successfulModelSyncAccountIDs(discoveryTask.data),
    [discoveryTask.data],
  );
  const discoveryStopped = taskStopsPolling(discoveryTask.data);
  const preview = useQuery({
    queryKey: ["account-model-sync-preview", discoveredAccountIDs],
    queryFn: () => api.previewAccountModels(discoveredAccountIDs),
    enabled: discoveryStopped && discoveredAccountIDs.length > 0 && applyTaskId === null,
    retry: false,
  });
  const apply = useMutation({
    mutationFn: () => {
      return api.applyAccountModels(
        accountSelections,
        preview.data?.fingerprint ?? "",
        probeModels,
      );
    },
    onMutate: () => setCatalogRefreshMessage(null),
    onSuccess: (task) => setApplyTaskId(task.id),
    onError: async (error) => {
      if (!isModelCatalogChangedError(error)) return;
      setCatalogRefreshMessage("检测到模型目录更新，正在载入最新结果...");
      const refreshed = await preview.refetch();
      if (refreshed.isSuccess) {
        setCatalogRefreshMessage("模型目录刚刚更新，已载入最新结果，请确认后再次同步。");
        return;
      }
      setCatalogRefreshMessage("模型目录已更新，但最新结果载入失败，请稍后重试。");
    },
  });
  const applyTask = useQuery({
    queryKey: ["account-model-apply", applyTaskId],
    queryFn: () => api.task(applyTaskId!),
    enabled: Boolean(applyTaskId),
    refetchInterval: taskPollInterval,
  });
  const pending =
    discovery.isPending ||
    (discoveryTaskId !== null && !discoveryStopped) ||
    apply.isPending ||
    (applyTaskId !== null && !taskStopsPolling(applyTask.data));
  let cancellableTaskId: string | null = null;
  if (applyTaskId !== null && !taskStopsPolling(applyTask.data)) {
    cancellableTaskId = applyTaskId;
  } else if (discoveryTaskId !== null && !discoveryStopped) {
    cancellableTaskId = discoveryTaskId;
  }

  useEffect(() => {
    if (!props.open) {
      startedTarget.current = null;
      initializedFingerprint.current = null;
      completedTask.current = null;
      setDiscoveryTaskId(null);
      setApplyTaskId(null);
      setScopeExclusions(new Map());
      setProbeModels([]);
      setCatalogRefreshMessage(null);
      discovery.reset();
      apply.reset();
      return;
    }
    if (props.accountIds.length === 0 || startedTarget.current === targetKey) return;
    startedTarget.current = targetKey;
    discovery.mutate();
  }, [props.open, targetKey]);

  useEffect(() => {
    if (!preview.data || initializedFingerprint.current === preview.data.fingerprint) return;
    initializedFingerprint.current = preview.data.fingerprint;
    const initial = new Map<string, Set<string>>();
    setScopeExclusions(initial);
    const selections = selectedAccountModels(
      preview.data,
      props.accountPlatforms,
      props.accountGroups,
      initial,
    );
    const firstProbeModel = availableUnifiedProbeModels(selections)[0];
    setProbeModels(firstProbeModel ? [firstProbeModel] : []);
  }, [preview.data, props.accountPlatforms, props.accountGroups]);

  useEffect(() => {
    if (!applyTask.data || !taskStopsPolling(applyTask.data)) return;
    if (completedTask.current === applyTask.data.id) return;
    completedTask.current = applyTask.data.id;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["accounts"] }),
      queryClient.invalidateQueries({ queryKey: ["policy"] }),
      queryClient.invalidateQueries({ queryKey: ["logs"] }),
    ]);
    props.onCompleted();
  }, [applyTask.data?.id, applyTask.data?.status, queryClient]);

  function setModelExcluded(scope: string, model: string, excluded: boolean): void {
    const next = new Map(scopeExclusions);
    const scopeModels = new Set(next.get(scope) ?? []);
    const modelKey = model.toLocaleLowerCase();
    if (excluded) scopeModels.add(modelKey);
    else scopeModels.delete(modelKey);
    if (scopeModels.size > 0) next.set(scope, scopeModels);
    else next.delete(scope);
    setScopeExclusions(next);
    if (!preview.data) return;
    const selections = selectedAccountModels(
      preview.data,
      props.accountPlatforms,
      props.accountGroups,
      next,
    );
    const available = availableUnifiedProbeModels(selections);
    const availableKeys = new Set(available.map((candidate) => candidate.toLocaleLowerCase()));
    const nextProbeModels = probeModels.filter((model) =>
      availableKeys.has(model.toLocaleLowerCase()),
    );
    if (nextProbeModels.length > 0) {
      setProbeModels(nextProbeModels);
      return;
    }
    setProbeModels(available[0] ? [available[0]] : []);
  }

  const accountSelections = preview.data
    ? selectedAccountModels(
        preview.data,
        props.accountPlatforms,
        props.accountGroups,
        scopeExclusions,
      )
    : [];
  const probeModelOptions = availableUnifiedProbeModels(accountSelections);
  const includedModelCount = accountSelections.reduce(
    (total, selection) => total + selection.models.length,
    0,
  );
  const probeModelOptionKeys = new Set(
    probeModelOptions.map((candidate) => candidate.toLocaleLowerCase()),
  );
  const missingProbeModel =
    probeModels.length === 0 ||
    probeModels.some((model) => !probeModelOptionKeys.has(model.toLocaleLowerCase()));
  const hasCompletedApply = Boolean(applyTask.data && taskStopsPolling(applyTask.data));
  let dialogStage: ModelSyncDialogStage = "pending";
  if (hasCompletedApply) dialogStage = "result";
  else if (preview.data && !applyTaskId) dialogStage = "selection";
  const dialogLayout = accountModelSyncDialogLayout(dialogStage);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open && pending) return;
        props.onOpenChange(open);
      }}
    >
      <DialogContent
        width={dialogLayout.width}
        height={dialogLayout.height}
        showCloseButton={!pending}
        className={dialogLayout.content}
      >
        <DialogHeader className="min-w-0 pr-8">
          <DialogTitle>批量同步账号模型</DialogTitle>
          <DialogDescription>
            {hasCompletedApply
              ? "查看本次模型写入和探活验证结果。"
              : "选择统一探活模型，再按平台和分组选择要同步的模型。"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid min-h-0 gap-4 overflow-y-auto">
          <ModelSyncDiscoveryState
            requestedCount={props.accountIds.length}
            mutationPending={discovery.isPending}
            mutationError={discovery.error}
            queryError={discoveryTask.error}
            task={discoveryTask.data}
          />
          {preview.isLoading ? <TaskStartupState message="正在生成模型同步预览" /> : null}
          {preview.error ? (
            <ModelSyncError error={preview.error} fallback="模型预览读取失败" />
          ) : null}
          {preview.data && !applyTaskId ? (
            <SyncModelEditor
              preview={preview.data}
              accountPlatforms={props.accountPlatforms}
              accountGroups={props.accountGroups}
              scopeExclusions={scopeExclusions}
              probeModels={probeModels}
              probeModelOptions={probeModelOptions}
              disabled={apply.isPending}
              onProbeModelsChange={setProbeModels}
              onModelExcluded={setModelExcluded}
            />
          ) : null}
          {catalogRefreshMessage ? (
            <p
              className="rounded-md border border-primary/30 bg-primary/5 px-3 py-2 text-sm"
              role="status"
            >
              {catalogRefreshMessage}
            </p>
          ) : null}
          {apply.isPending ? <TaskStartupState message="正在创建模型应用与探测任务" /> : null}
          {apply.error && !isModelCatalogChangedError(apply.error) ? (
            <ModelSyncError error={apply.error} fallback="模型应用任务启动失败" />
          ) : null}
          {applyTask.error ? (
            <ModelSyncError error={applyTask.error} fallback="模型应用状态读取失败" />
          ) : null}
          {applyTask.data ? <ModelSyncApplyState task={applyTask.data} /> : null}
        </DialogBody>
        <DialogFooter>
          {cancellableTaskId ? <TaskCancelButton taskId={cancellableTaskId} /> : null}
          <Button variant="outline" disabled={pending} onClick={() => props.onOpenChange(false)}>
            关闭
          </Button>
          {preview.data && !applyTaskId ? (
            <Button
              disabled={includedModelCount === 0 || missingProbeModel || apply.isPending}
              onClick={() => apply.mutate()}
              aria-label={`同步 ${discoveredAccountIDs.length} 个账号`}
            >
              <CheckCheck aria-hidden="true" />
              同步 {discoveredAccountIDs.length} 个账号
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ModelSyncDiscoveryState(props: {
  requestedCount: number;
  mutationPending: boolean;
  mutationError: unknown;
  queryError: unknown;
  task: Task | undefined;
}) {
  if (props.mutationPending) return <TaskStartupState message="正在创建账号模型发现任务" />;
  if (props.mutationError)
    return <ModelSyncError error={props.mutationError} fallback="模型发现启动失败" />;
  if (props.queryError)
    return <ModelSyncError error={props.queryError} fallback="模型发现状态读取失败" />;
  if (!props.task)
    return <TaskStartupState message={`准备获取 ${props.requestedCount} 个账号的模型目录`} />;
  if (!taskStopsPolling(props.task)) {
    return <TaskProgressState message={props.task.message} progress={props.task.progress} />;
  }
  if (props.task.status === "failed" || props.task.status === "cancelled") {
    return <ModelSyncError error={props.task.message} fallback="账号模型发现未完成" />;
  }
  const failed = modelSyncTaskItems(props.task).filter((item) => item.status !== "succeeded");
  if (failed.length === 0) return null;
  return (
    <details
      className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
      aria-live="polite"
    >
      <summary className="cursor-pointer text-destructive">
        <strong>{failed.length} 个账号获取模型失败，已跳过</strong>
        <span className="ml-2 text-xs">展开详情</span>
      </summary>
      <div className="mt-2 grid max-h-40 gap-1 overflow-y-auto border-t border-destructive/20 pt-2 text-xs text-destructive">
        {failed.map((item) => (
          <span className="break-words" key={item.accountId}>
            {item.accountName || `账号 #${item.accountId}`}：{item.error || "模型发现失败"}
          </span>
        ))}
      </div>
    </details>
  );
}

function SyncModelEditor(props: {
  preview: AccountModelSyncPreview;
  accountPlatforms: ReadonlyMap<string, string | null>;
  accountGroups: ReadonlyMap<string, string[]>;
  scopeExclusions: ScopeModelExclusions;
  probeModels: string[];
  probeModelOptions: string[];
  disabled: boolean;
  onProbeModelsChange: (models: string[]) => void;
  onModelExcluded: (scope: string, model: string, excluded: boolean) => void;
}) {
  const platforms = useMemo(
    () => modelSyncPlatformGroups(props.preview, props.accountPlatforms, props.accountGroups),
    [props.preview, props.accountPlatforms, props.accountGroups],
  );
  const [activePlatform, setActivePlatform] = useState(platforms[0]?.key ?? "");
  const [activeGroup, setActiveGroup] = useState(platforms[0]?.groups[0]?.key ?? "");
  const [modelSearch, setModelSearch] = useState("");
  const currentPlatform =
    platforms.find((platform) => platform.key === activePlatform) ?? platforms[0];
  const currentGroup =
    currentPlatform?.groups.find((group) => group.key === activeGroup) ??
    currentPlatform?.groups[0];
  const blockedModels = new Set(
    props.preview.blocked_models.map((model) => model.toLocaleLowerCase()),
  );
  const selectableModels =
    currentGroup?.models.filter((item) => !blockedModels.has(item.model.toLocaleLowerCase())) ?? [];
  const visibleModels = selectableModels.filter((item) =>
    item.model.toLocaleLowerCase().includes(modelSearch.trim().toLocaleLowerCase()),
  );
  const currentScopeExclusions = currentGroup
    ? props.scopeExclusions.get(currentGroup.scopeKey)
    : undefined;
  const selectedInGroup = selectableModels.filter(
    (item) => !currentScopeExclusions?.has(item.model.toLocaleLowerCase()),
  ).length;

  useEffect(() => {
    if (platforms.some((platform) => platform.key === activePlatform)) return;
    setActivePlatform(platforms[0]?.key ?? "");
    setActiveGroup(platforms[0]?.groups[0]?.key ?? "");
  }, [activePlatform, platforms]);

  useEffect(() => {
    if (currentPlatform?.groups.some((group) => group.key === activeGroup)) return;
    setActiveGroup(currentPlatform?.groups[0]?.key ?? "");
  }, [activeGroup, currentPlatform]);

  return (
    <section
      className="grid min-h-0 content-start gap-3"
      aria-labelledby="account-sync-models-title"
    >
      <div
        className="grid self-start gap-3 rounded-lg border p-4"
        data-testid="unified-probe-models"
      >
        <FieldLabel
          label="统一探活模型"
          description="最多选择 20 个；同步完成后依次验证，不支持的账号自动跳过。"
        />
        {props.probeModelOptions.length > 0 ? (
          <MultiSelect
            options={props.probeModelOptions.map((model) => ({ value: model, label: model }))}
            selected={props.probeModels}
            onChange={(models) => props.onProbeModelsChange(models.slice(0, 20))}
            title="选择探活模型"
            searchPlaceholder="搜索探活模型"
            emptyText="没有匹配的探活模型"
            clearText="清空探活模型"
            ariaLabel="统一探活模型"
            maxVisibleChips={4}
            disabled={props.disabled}
            className="w-full sm:max-w-2xl"
          />
        ) : (
          <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
            请至少勾选一个要同步的模型。
          </p>
        )}
      </div>

      <div className="grid min-h-0 content-start gap-3 rounded-lg border p-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h3 className="font-semibold" id="account-sync-models-title">
              选择要同步的模型
            </h3>
          </div>
          <SearchInput
            label="搜索模型"
            placeholder="搜索当前平台模型"
            value={modelSearch}
            onChange={setModelSearch}
          />
        </div>

        {platforms.length > 0 ? (
          <SegmentedControl role="tablist" aria-label="账号平台" className="overflow-x-auto">
            {platforms.map((platform) => (
              <SegmentedControlItem
                key={platform.key}
                role="tab"
                selected={platform.key === currentPlatform?.key}
                aria-controls="account-model-sync-platform-panel"
                onClick={() => {
                  setActivePlatform(platform.key);
                  setActiveGroup(platform.groups[0]?.key ?? "");
                  setModelSearch("");
                }}
              >
                {platform.label} · {platform.accountCount}
              </SegmentedControlItem>
            ))}
          </SegmentedControl>
        ) : null}

        {currentPlatform ? (
          <div
            className="grid min-h-0 gap-3"
            id="account-model-sync-platform-panel"
            role="tabpanel"
          >
            <SegmentedControl role="tablist" aria-label="账号分组" className="overflow-x-auto">
              {currentPlatform.groups.map((group) => (
                <SegmentedControlItem
                  key={group.key}
                  role="tab"
                  selected={group.key === currentGroup?.key}
                  aria-controls="account-model-sync-group-panel"
                  onClick={() => {
                    setActiveGroup(group.key);
                    setModelSearch("");
                  }}
                >
                  {group.label} · {group.accountCount}
                </SegmentedControlItem>
              ))}
            </SegmentedControl>
            {currentGroup ? (
              <div
                className="grid min-h-0 gap-3"
                id="account-model-sync-group-panel"
                role="tabpanel"
              >
                <div className="flex items-center justify-between gap-3 text-xs">
                  <span className="text-muted-foreground">
                    已选 {selectedInGroup}/{selectableModels.length} 个模型
                  </span>
                  <span>{currentGroup.accountCount} 个账号</span>
                </div>
                <div
                  className="grid max-h-[30rem] auto-rows-[3.5rem] content-start grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
                  data-testid="account-sync-models"
                >
                  {visibleModels.map((item) => {
                    const included = !currentScopeExclusions?.has(item.model.toLocaleLowerCase());
                    return (
                      <div
                        className={cn(
                          "flex min-w-0 items-start gap-2 rounded-md border bg-background px-3 py-2.5 transition-colors",
                          props.disabled
                            ? "cursor-not-allowed"
                            : "cursor-pointer hover:border-primary/50 hover:bg-muted/40",
                          included && "border-primary/40",
                        )}
                        key={item.model}
                        role="group"
                        aria-label={`模型 ${item.model}`}
                        aria-disabled={props.disabled}
                        onClick={() => {
                          if (props.disabled) return;
                          props.onModelExcluded(currentGroup.scopeKey, item.model, included);
                        }}
                      >
                        <Checkbox
                          className="mt-0.5"
                          checked={included}
                          disabled={props.disabled}
                          onCheckedChange={(checked) =>
                            props.onModelExcluded(
                              currentGroup.scopeKey,
                              item.model,
                              checked !== true,
                            )
                          }
                          onClick={(event) => event.stopPropagation()}
                          aria-label={`同步模型 ${item.model}`}
                        />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate font-mono text-xs">{item.model}</span>
                          <span className="text-muted-foreground mt-1 block text-xs">
                            {item.accountCount}/{currentGroup.accountCount} 个账号支持
                          </span>
                        </span>
                      </div>
                    );
                  })}
                  {visibleModels.length === 0 ? (
                    <p className="text-muted-foreground col-span-full flex h-14 items-center justify-center px-3 text-center text-sm">
                      没有匹配的模型
                    </p>
                  ) : null}
                </div>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  );
}

type ModelSyncPlatformGroup = {
  key: string;
  label: string;
  accountCount: number;
  groups: ModelSyncGroup[];
};

type ModelSyncGroup = {
  key: string;
  scopeKey: string;
  label: string;
  accountCount: number;
  models: Array<{ model: string; accountCount: number }>;
};

function modelSyncPlatformGroups(
  preview: AccountModelSyncPreview,
  accountPlatforms: ReadonlyMap<string, string | null>,
  accountGroups: ReadonlyMap<string, string[]>,
): ModelSyncPlatformGroup[] {
  const groups = new Map<
    string,
    {
      label: string;
      accountCount: number;
      groups: Map<
        string,
        {
          label: string;
          accountCount: number;
          models: Map<string, { model: string; accountCount: number }>;
        }
      >;
    }
  >();
  for (const account of preview.accounts) {
    const key = accountPlatformKey(account, accountPlatforms);
    const platform = key === "unclassified" ? "" : key;
    let group = groups.get(key);
    if (!group) {
      group = {
        label: accountPlatformLabel(platform) ?? "未识别平台",
        accountCount: 0,
        groups: new Map(),
      };
      groups.set(key, group);
    }
    group.accountCount++;
    const memberships = uniqueAccountGroups(accountGroups.get(account.account_id));
    for (const membership of memberships) {
      const groupKey = membership.toLocaleLowerCase();
      let membershipGroup = group.groups.get(groupKey);
      if (!membershipGroup) {
        membershipGroup = { label: membership, accountCount: 0, models: new Map() };
        group.groups.set(groupKey, membershipGroup);
      }
      membershipGroup.accountCount++;
      for (const model of account.models) {
        const modelKey = model.toLocaleLowerCase();
        const coverage = membershipGroup.models.get(modelKey);
        if (coverage) coverage.accountCount++;
        else membershipGroup.models.set(modelKey, { model, accountCount: 1 });
      }
    }
  }
  return Array.from(groups, ([key, group]) => ({
    key,
    label: group.label,
    accountCount: group.accountCount,
    groups: Array.from(group.groups, ([groupKey, membership]) => ({
      key: groupKey,
      scopeKey: modelSyncScopeKey(key, membership.label),
      label: membership.label,
      accountCount: membership.accountCount,
      models: Array.from(membership.models.values()).sort((left, right) =>
        left.model.localeCompare(right.model),
      ),
    })).sort((left, right) => left.label.localeCompare(right.label)),
  })).sort((left, right) => left.label.localeCompare(right.label));
}

function uniqueAccountGroups(groups: string[] | undefined): string[] {
  const result = new Map<string, string>();
  for (const rawGroup of groups ?? []) {
    const group = rawGroup.trim();
    if (group) result.set(group.toLocaleLowerCase(), group);
  }
  return result.size > 0 ? Array.from(result.values()) : ["未分组"];
}

function SearchInput(props: {
  label: string;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="relative block sm:max-w-72">
      <span className="sr-only">{props.label}</span>
      <Search
        className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
        aria-hidden="true"
      />
      <Input
        className="pl-9"
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
        placeholder={props.placeholder}
      />
    </label>
  );
}

function ModelSyncApplyState(props: { task: Task }) {
  if (!taskStopsPolling(props.task)) {
    return <TaskProgressState message={props.task.message} progress={props.task.progress} />;
  }
  return <ModelSyncCompletedState task={props.task} />;
}

type ProbeResultFilter = "failed" | "skipped" | "passed";

function ModelSyncCompletedState(props: { task: Task }) {
  const taskItems = modelSyncTaskItems(props.task);
  const applicationIssues = taskItems.filter((item) => item.status !== "succeeded");
  const accountNames = new Map(taskItems.map((item) => [item.accountId, item.accountName]));
  const probeItems = modelSyncProbeItems(props.task);
  const probeGroups: Record<ProbeResultFilter, ModelSyncProbeItem[]> = {
    failed: probeItems.filter((item) => item.result !== "通过" && item.result !== "跳过"),
    skipped: probeItems.filter((item) => item.result === "跳过"),
    passed: probeItems.filter((item) => item.result === "通过"),
  };
  const [filter, setFilter] = useState<ProbeResultFilter>(() => {
    if (probeGroups.failed.length > 0) return "failed";
    if (probeGroups.skipped.length > 0) return "skipped";
    return "passed";
  });
  const applied = taskResultCount(
    props.task.result.applied,
    taskItems.filter((item) => item.status === "succeeded").length,
  );
  const skipped = taskResultCount(
    props.task.result.skipped,
    taskItems.filter((item) => item.status === "skipped").length,
  );
  const failed = taskResultCount(props.task.result.failed, applicationIssues.length - skipped);
  const probeError =
    typeof props.task.result.probe_error === "string" ? props.task.result.probe_error : "";
  const partial =
    props.task.status !== "succeeded" ||
    applicationIssues.length > 0 ||
    probeGroups.failed.length > 0 ||
    probeError !== "";
  let resultMessage = "模型写入和探活验证均已完成";
  if (partial) resultMessage = "操作已完成，部分结果需要关注";
  if (props.task.status === "failed" || props.task.status === "cancelled") {
    resultMessage = props.task.message || "模型同步未完成，请查看任务详情";
  }
  const ResultIcon = partial ? CircleAlert : CheckCircle2;
  return (
    <section
      className="flex h-full min-h-0 flex-col gap-4 overflow-hidden"
      data-testid="model-sync-apply-result"
      aria-live="polite"
    >
      <header className="flex items-start gap-3 border-b pb-4">
        <span
          className={cn(
            "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full",
            partial ? "bg-warning/10 text-warning" : "bg-success/10 text-success",
          )}
        >
          <ResultIcon className="size-4" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <h3 className="font-semibold" id="model-sync-result-title">
            同步结果
          </h3>
          <p className="text-muted-foreground mt-1 text-sm">{resultMessage}</p>
        </div>
      </header>

      <div className="grid gap-4 lg:grid-cols-2">
        <ResultSummary
          title="模型写入"
          values={[
            { label: "写入", value: applied, tone: "success" },
            { label: "跳过", value: skipped, tone: "neutral" },
            { label: "失败", value: failed, tone: "danger" },
          ]}
        />
        <ResultSummary
          title="探活验证"
          values={[
            { label: "通过", value: probeGroups.passed.length, tone: "success" },
            { label: "跳过", value: probeGroups.skipped.length, tone: "neutral" },
            { label: "失败", value: probeGroups.failed.length, tone: "danger" },
          ]}
        />
      </div>

      {applicationIssues.length > 0 ? (
        <section className="grid gap-2" aria-labelledby="model-sync-write-issues-title">
          <h4 className="text-sm font-medium" id="model-sync-write-issues-title">
            写入异常
          </h4>
          <div className="max-h-36 divide-y overflow-y-auto rounded-md border">
            {applicationIssues.map((item) => (
              <div className="flex items-start gap-3 px-3 py-2.5 text-sm" key={item.accountId}>
                <XCircle className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden="true" />
                <div className="min-w-0">
                  <div className="font-medium break-words">
                    {item.accountName || `账号 #${item.accountId}`}
                  </div>
                  <div className="text-muted-foreground mt-0.5 break-words">
                    {item.error || "模型写入失败"}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {probeError ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive break-words">
          探活任务未完整执行：{probeError}
        </p>
      ) : null}

      <section
        className="flex min-h-0 flex-1 flex-col gap-3"
        aria-labelledby="model-sync-probe-details-title"
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h4 className="text-sm font-medium" id="model-sync-probe-details-title">
            探活明细
          </h4>
          <SegmentedControl role="tablist" aria-label="探活结果分类">
            <SegmentedControlItem
              role="tab"
              selected={filter === "failed"}
              aria-controls="model-sync-probe-result-list"
              onClick={() => setFilter("failed")}
            >
              失败 {probeGroups.failed.length}
            </SegmentedControlItem>
            <SegmentedControlItem
              role="tab"
              selected={filter === "skipped"}
              aria-controls="model-sync-probe-result-list"
              onClick={() => setFilter("skipped")}
            >
              跳过 {probeGroups.skipped.length}
            </SegmentedControlItem>
            <SegmentedControlItem
              role="tab"
              selected={filter === "passed"}
              aria-controls="model-sync-probe-result-list"
              onClick={() => setFilter("passed")}
            >
              通过 {probeGroups.passed.length}
            </SegmentedControlItem>
          </SegmentedControl>
        </div>
        <div
          className="min-h-24 flex-1 overflow-y-auto rounded-md border"
          id="model-sync-probe-result-list"
          role="tabpanel"
        >
          {probeGroups[filter].map((item) => {
            const accountName = accountNames.get(item.accountId) || `账号 #${item.accountId}`;
            const model = item.requestModel || "未配置探测模型";
            const skipped = item.result === "跳过";
            const passed = item.result === "通过";
            let ItemIcon = XCircle;
            if (skipped) ItemIcon = CircleSlash2;
            else if (passed) ItemIcon = CheckCircle2;
            return (
              <div
                className="flex min-w-0 items-start gap-3 border-b px-3 py-2.5 last:border-b-0"
                key={`${item.accountId}-${item.groupName}-${item.requestModel}`}
              >
                <ItemIcon
                  className={cn(
                    "mt-0.5 size-4 shrink-0",
                    skipped && "text-muted-foreground",
                    passed && "text-success",
                    !skipped && !passed && "text-destructive",
                  )}
                  aria-hidden="true"
                />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                    <span className="font-medium break-words">{accountName}</span>
                    <code className="bg-muted max-w-full rounded px-1.5 py-0.5 text-xs break-all">
                      {model}
                    </code>
                  </div>
                  <p
                    className={cn(
                      "mt-1 text-xs break-words",
                      passed ? "text-success" : "text-muted-foreground",
                    )}
                  >
                    {modelSyncProbeReason(item)}
                  </p>
                </div>
              </div>
            );
          })}
          {probeGroups[filter].length === 0 ? (
            <p className="text-muted-foreground px-3 py-8 text-center text-sm">当前分类没有结果</p>
          ) : null}
        </div>
      </section>
    </section>
  );
}

type ResultTone = "success" | "neutral" | "danger";

function ResultSummary(props: {
  title: string;
  values: Array<{ label: string; value: number; tone: ResultTone }>;
}) {
  return (
    <section className="grid gap-2" aria-label={`${props.title}结果`}>
      <h4 className="text-muted-foreground text-xs font-medium">{props.title}</h4>
      <dl className="grid grid-cols-3 divide-x rounded-md border">
        {props.values.map((item) => (
          <div className="grid gap-0.5 px-3 py-2.5" key={item.label}>
            <dt className="text-muted-foreground text-xs">{item.label}</dt>
            <dd
              className={cn(
                "text-lg font-semibold tabular-nums",
                item.tone === "success" && "text-success",
                item.tone === "neutral" && "text-foreground",
                item.tone === "danger" && item.value > 0 && "text-destructive",
              )}
            >
              {item.value}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function taskResultCount(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : fallback;
}

function modelSyncProbeReason(item: ModelSyncProbeItem): string {
  if (item.result === "通过") return "验证通过";
  const message = item.error.trim();
  if (message.includes("所选实际验证模型不在该账号的已启用模型中")) {
    return "该账号未启用此模型";
  }
  const apiError = /^API returned (\d{3})(?::\s*([\s\S]*))?$/i.exec(message);
  if (!apiError) return message || item.result;
  const status = apiError[1];
  const upstreamMessage = apiError[2] ? upstreamAPIErrorMessage(apiError[2]) : "";
  if (upstreamMessage.toLocaleLowerCase() === "service temporarily unavailable") {
    return `上游服务暂时不可用（HTTP ${status}）`;
  }
  return upstreamMessage
    ? `上游返回 HTTP ${status}：${upstreamMessage}`
    : `上游返回 HTTP ${status}`;
}

function upstreamAPIErrorMessage(raw: string): string {
  try {
    const payload = JSON.parse(raw) as unknown;
    if (typeof payload !== "object" || payload === null || Array.isArray(payload)) return raw;
    const error = (payload as Record<string, unknown>).error;
    if (typeof error !== "object" || error === null || Array.isArray(error)) return raw;
    const message = (error as Record<string, unknown>).message;
    return typeof message === "string" && message.trim() ? message.trim() : raw;
  } catch {
    return raw;
  }
}

function ModelSyncError(props: { error: unknown; fallback: string }) {
  return (
    <p className="break-words text-sm text-destructive" role="alert">
      {operationErrorMessage(props.error, props.fallback)}
    </p>
  );
}
