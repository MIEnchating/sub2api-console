import { DictionaryReadState } from "@/components/dictionary-read-state";
import { useDictionary } from "@/hooks/use-dictionary-order";
import { QueryErrorToast } from "@/components/query-error-toast";
import { orderByDictionary } from "@/lib/dictionary-order";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCheck,
  CheckCircle2,
  CircleAlert,
  CircleSlash2,
  Ban,
  Search,
  XCircle,
} from "lucide-react";

import {
  api,
  ApiError,
  type AccountModelSelection,
  type AccountModelSyncAccount,
  type AccountModelSyncPreview,
  type DictionaryEntry,
  type Task,
} from "@/api";
import { FieldLabel } from "@/components/field-help-tooltip";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { accountPlatformLabel } from "@/features/accounts/lib/account-labels";
import { orderedDictionaryOptions } from "@/lib/domain-dictionaries";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { cn } from "@/lib/utils";
import { ManualSyncModelForm } from "./manual-sync-model-form";
import { ModelSyncScopeDialog } from "./model-sync-scope-dialog";
import { useModelSyncBlocking } from "../hooks/use-model-sync-blocking";
import { modelMatchesBlockPatterns } from "../lib/model-block-patterns";
import { includedGroupModelKeys } from "../lib/model-sync-selection";
import {
  type ModelSyncModel,
  type ModelSyncPlatformGroup,
  type ModelSyncSupportingAccount,
} from "../lib/model-sync-groups";

type ModelSyncTaskItem = {
  accountId: string;
  accountName: string;
  status: string;
  error: string;
};

type ModelSyncUpstream = {
  upstreamId: string | null;
  upstreamHost: string | null;
};

type ModelSyncFailureGroup = {
  key: string;
  label: string;
  items: ModelSyncTaskItem[];
  errors: Array<{ message: string; items: ModelSyncTaskItem[] }>;
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
  inclusions: ScopeModelExclusions = new Map(),
): AccountModelSelection[] {
  const blocked = new Set(preview.blocked_models.map((model) => model.toLocaleLowerCase()));
  return preview.accounts.map((account) => {
    const platform = accountPlatformKey(account, accountPlatforms);
    const scopes = uniqueAccountGroups(accountGroups.get(account.account_id)).map((group) =>
      modelSyncScopeKey(platform, group),
    );
    const enabled = new Set(
      (account.enabled_models ?? []).map((model) => model.toLocaleLowerCase()),
    );
    const models = account.models.filter((model) => {
      const modelKey = model.toLocaleLowerCase();
      if (blocked.has(modelKey) || modelMatchesBlockPatterns(model, preview.blocked_patterns))
        return false;
      return scopes.some(
        (scope) =>
          !exclusions.get(scope)?.has(modelKey) &&
          (inclusions.get(scope)?.has(modelKey) || enabled.has(modelKey)),
      );
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

export function groupModelSyncFailures(
  items: ModelSyncTaskItem[],
  accountUpstreams: ReadonlyMap<string, ModelSyncUpstream>,
): ModelSyncFailureGroup[] {
  const groups = new Map<string, ModelSyncFailureGroup>();
  for (const item of items) {
    const upstream = accountUpstreams.get(item.accountId);
    const upstreamId = upstream?.upstreamId?.trim();
    const upstreamHost = upstream?.upstreamHost?.trim();
    const key = upstreamId || upstreamHost || `account:${item.accountId}`;
    const label =
      upstreamHost ||
      (upstreamId ? `上游 ${upstreamId}` : item.accountName || `账号 #${item.accountId}`);
    const group = groups.get(key);
    if (group) {
      group.items.push(item);
      const message = item.error || "模型发现失败";
      const matchingError = group.errors.find((error) => error.message === message);
      if (matchingError) matchingError.items.push(item);
      else group.errors.push({ message, items: [item] });
    } else {
      groups.set(key, {
        key,
        label,
        items: [item],
        errors: [{ message: item.error || "模型发现失败", items: [item] }],
      });
    }
  }
  return Array.from(groups.values());
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

type AccountModelSyncDialogProps = {
  open: boolean;
  accountIds: string[];
  accountPlatforms: ReadonlyMap<string, string | null>;
  accountGroups: ReadonlyMap<string, string[]>;
  accountUpstreams?: ReadonlyMap<string, ModelSyncUpstream>;
  onOpenChange: (open: boolean) => void;
  onCompleted: () => void;
};

export function AccountModelSyncDialog(props: AccountModelSyncDialogProps) {
  if (!props.open) return null;
  return <AccountModelSyncSession key={props.accountIds.join("\u0000")} {...props} />;
}

function AccountModelSyncSession(props: AccountModelSyncDialogProps) {
  const queryClient = useQueryClient();
  const [accountIds, setAccountIds] = useState<string[] | null>(null);
  const starting = useRef(false);
  const discovery = useMutation({
    mutationFn: api.discoverAccountModels,
    retry: false,
    networkMode: "always",
    onSuccess: (task) => {
      queryClient.setQueryData(["account-model-discovery", task.id], task);
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
    },
  });
  if (accountIds === null)
    return (
      <ModelSyncScopeDialog
        accountIds={props.accountIds}
        accountGroups={props.accountGroups}
        onOpenChange={props.onOpenChange}
        onStart={(ids) => {
          if (starting.current) return;
          starting.current = true;
          discovery.mutate(ids);
          setAccountIds(ids);
        }}
      />
    );
  return (
    <AccountModelSyncRunDialog
      {...props}
      accountIds={accountIds}
      discoveryTaskId={discovery.data?.id ?? null}
      discoveryPending={discovery.isPending}
      discoveryError={discovery.error}
    />
  );
}

function AccountModelSyncRunDialog(
  props: AccountModelSyncDialogProps & {
    discoveryTaskId: string | null;
    discoveryPending: boolean;
    discoveryError: unknown;
  },
) {
  // Keep draft scopes stable when the account directory refreshes.
  const [membership] = useState(() => ({
    platforms: new Map(props.accountIds.map((id) => [id, props.accountPlatforms.get(id) ?? null])),
    groups: new Map(props.accountIds.map((id) => [id, [...(props.accountGroups.get(id) ?? [])]])),
  }));
  const membershipChanged = props.accountIds.some((id) => {
    const initialPlatform = (membership.platforms.get(id) ?? "").trim().toLocaleLowerCase();
    const currentPlatform = (props.accountPlatforms.get(id) ?? "").trim().toLocaleLowerCase();
    const initialGroups = uniqueAccountGroups(membership.groups.get(id)).map((group) =>
      group.toLocaleLowerCase(),
    );
    const currentGroups = new Set(
      uniqueAccountGroups(props.accountGroups.get(id)).map((group) => group.toLocaleLowerCase()),
    );
    return (
      initialPlatform !== currentPlatform ||
      initialGroups.length !== currentGroups.size ||
      initialGroups.some((group) => !currentGroups.has(group))
    );
  });
  const queryClient = useQueryClient();
  const platformDictionary = useDictionary("platform");
  const groupDictionary = useDictionary("group");
  const discoveryTaskId = props.discoveryTaskId;
  const [applyTaskId, setApplyTaskId] = useState<string | null>(null);
  const [scopeExclusions, setScopeExclusions] = useState<Map<string, Set<string>>>(() => new Map());
  const [scopeInclusions, setScopeInclusions] = useState<Map<string, Set<string>>>(() => new Map());
  const [probeModels, setProbeModels] = useState<string[]>([]);
  const [manualModels, setManualModels] = useState<Map<string, string[]>>(() => new Map());
  const [catalogRefreshMessage, setCatalogRefreshMessage] = useState<string | null>(null);
  const initializedFingerprint = useRef<string | null>(null);
  const completedTask = useRef<string | null>(null);
  const mounted = useRef(false);
  const submitting = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const discoveryTask = useQuery({
    queryKey: ["account-model-discovery", discoveryTaskId],
    queryFn: ({ signal }) => api.task(discoveryTaskId!, signal),
    enabled: Boolean(discoveryTaskId),
    refetchInterval: taskPollInterval,
  });
  const discoveredAccountIDs = useMemo(
    () => successfulModelSyncAccountIDs(discoveryTask.data),
    [discoveryTask.data],
  );
  const discoveryStopped = taskStopsPolling(discoveryTask.data);
  const blocking = useModelSyncBlocking({
    accountIds: discoveredAccountIDs,
    onPreviewUpdated: (updated) => {
      // A global block changes the fingerprint without discarding the user's draft.
      initializedFingerprint.current = updated.fingerprint;
      setProbeModels((current) =>
        current.filter((model) => !modelMatchesBlockPatterns(model, updated.blocked_patterns)),
      );
    },
  });
  const preview = useQuery({
    queryKey: ["account-model-sync-preview", discoveredAccountIDs],
    queryFn: ({ signal }) => api.previewAccountModels(discoveredAccountIDs, signal),
    enabled:
      discoveryStopped &&
      discoveredAccountIDs.length > 0 &&
      applyTaskId === null &&
      !blocking.pending &&
      blocking.savedSettings === null,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const apply = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: () => {
      if (membershipChanged) throw new Error("账号的平台或分组已变化，请关闭后重新选择同步范围。");
      if (!dictionariesReady || hasHiddenSelections) {
        throw new Error("部分账号的平台或分组已不在可选字典中，请刷新字典并重新预览");
      }
      return api.applyAccountModels(
        accountSelections.map((selection) => {
          const manual = manualModels.get(selection.account_id);
          if (!manual?.length) return selection;
          return {
            ...selection,
            manual_models: manual.filter((model) => selection.models.includes(model)),
          };
        }),
        preview.data?.fingerprint ?? "",
        probeModels,
      );
    },
    onMutate: () => setCatalogRefreshMessage(null),
    onSuccess: (task) => {
      queryClient.setQueryData(["account-model-apply", task.id], task);
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      if (mounted.current) setApplyTaskId(task.id);
    },
    onError: async (error) => {
      if (!mounted.current || !isModelCatalogChangedError(error)) return;
      setCatalogRefreshMessage("检测到模型目录更新，正在载入最新结果...");
      const refreshed = await preview.refetch();
      if (!mounted.current) return;
      if (refreshed.isSuccess) {
        setCatalogRefreshMessage("模型目录刚刚更新，已载入最新结果，请确认后再次同步。");
        return;
      }
      setCatalogRefreshMessage("模型目录已更新，但最新结果载入失败，请稍后重试。");
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  const applyTask = useQuery({
    queryKey: ["account-model-apply", applyTaskId],
    queryFn: ({ signal }) => api.task(applyTaskId!, signal),
    enabled: Boolean(applyTaskId),
    refetchInterval: taskPollInterval,
  });
  const pending =
    blocking.pending ||
    props.discoveryPending ||
    (discoveryTaskId !== null && !discoveryStopped && !discoveryTask.isError) ||
    apply.isPending ||
    (applyTaskId !== null && !taskStopsPolling(applyTask.data) && !applyTask.isError);
  let cancellableTaskId: string | null = null;
  if (applyTaskId !== null && !taskStopsPolling(applyTask.data)) {
    cancellableTaskId = applyTaskId;
  } else if (discoveryTaskId !== null && !discoveryStopped) {
    cancellableTaskId = discoveryTaskId;
  }

  useEffect(() => {
    if (!preview.data || initializedFingerprint.current === preview.data.fingerprint) return;
    initializedFingerprint.current = preview.data.fingerprint;
    const initial = new Map<string, Set<string>>();
    setScopeExclusions(initial);
    setScopeInclusions(new Map());
    setProbeModels([]);
    setManualModels(new Map());
  }, [preview.data]);

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

  function setModelExcluded(scopes: string[], model: string, excluded: boolean): void {
    const next = new Map(scopeExclusions);
    const nextInclusions = new Map(scopeInclusions);
    const modelKey = model.toLocaleLowerCase();
    for (const scope of scopes) {
      const scopeModels = new Set(next.get(scope) ?? []);
      const included = new Set(nextInclusions.get(scope) ?? []);
      if (excluded) {
        scopeModels.add(modelKey);
        included.delete(modelKey);
      } else {
        scopeModels.delete(modelKey);
        included.add(modelKey);
      }
      next.set(scope, scopeModels);
      nextInclusions.set(scope, included);
    }
    setScopeExclusions(next);
    setScopeInclusions(nextInclusions);
    if (!editorPreview) return;
    const selections = selectedAccountModels(
      editorPreview,
      membership.platforms,
      membership.groups,
      next,
      nextInclusions,
    );
    const available = availableUnifiedProbeModels(selections);
    const availableKeys = new Set(available.map((candidate) => candidate.toLocaleLowerCase()));
    const nextProbeModels = probeModels.filter((model) =>
      availableKeys.has(model.toLocaleLowerCase()),
    );
    setProbeModels(nextProbeModels);
  }

  const editorPreview = useMemo(
    () =>
      preview.data
        ? {
            ...preview.data,
            blocked_patterns:
              blocking.savedSettings?.blocked_patterns ?? preview.data.blocked_patterns,
            accounts: preview.data.accounts.map((account) => ({
              ...account,
              models: [
                ...new Set([...account.models, ...(manualModels.get(account.account_id) ?? [])]),
              ],
            })),
          }
        : undefined,
    [preview.data, blocking.savedSettings, manualModels],
  );
  const accountSelections = useMemo(
    () =>
      editorPreview
        ? selectedAccountModels(
            editorPreview,
            membership.platforms,
            membership.groups,
            scopeExclusions,
            scopeInclusions,
          )
        : [],
    [editorPreview, membership, scopeExclusions, scopeInclusions],
  );
  const platforms = useMemo(
    () =>
      editorPreview
        ? modelSyncPlatformGroups(
            editorPreview,
            membership.platforms,
            membership.groups,
            platformDictionary.data?.items,
            groupDictionary.data?.items,
          )
        : [],
    [editorPreview, membership, platformDictionary.data?.items, groupDictionary.data?.items],
  );
  const probeModelOptions = availableUnifiedProbeModels(accountSelections);
  const includedModelCount = accountSelections.reduce(
    (total, selection) => total + selection.models.length,
    0,
  );
  const probeModelOptionKeys = new Set(
    probeModelOptions.map((candidate) => candidate.toLocaleLowerCase()),
  );
  const missingProbeModel = probeModels.some(
    (model) => !probeModelOptionKeys.has(model.toLocaleLowerCase()),
  );
  const dictionariesReady = Boolean(platformDictionary.data && groupDictionary.data);
  const visibleAccountIDs = useMemo(
    () =>
      new Set(
        platforms.flatMap((platform) => platform.groups.flatMap((group) => group.accountIds)),
      ),
    [platforms],
  );
  const hasHiddenSelections = accountSelections.some(
    (selection) => !visibleAccountIDs.has(selection.account_id),
  );
  const editorDisabled =
    membershipChanged ||
    apply.isPending ||
    blocking.pending ||
    blocking.savedSettings !== null ||
    !dictionariesReady ||
    preview.isFetching ||
    preview.isError;
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
              : "按平台和分组确认同步模型；探活模型留空时不执行探活。"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid min-h-0 gap-4 overflow-y-auto">
          <ModelSyncDiscoveryState
            requestedCount={props.accountIds.length}
            mutationPending={props.discoveryPending}
            mutationError={props.discoveryError}
            queryError={discoveryTask.error}
            task={discoveryTask.data}
            accountUpstreams={props.accountUpstreams ?? new Map()}
          />
          {discoveryTask.isError ? (
            <ContentRetry
              onRetry={() => void discoveryTask.refetch()}
              pending={discoveryTask.isFetching}
            />
          ) : null}
          {preview.isLoading ? <ContentLoading label="正在生成模型同步预览" /> : null}
          {preview.error ? (
            <>
              <ModelSyncError error={preview.error} fallback="模型预览读取失败" />
              <ContentRetry onRetry={() => void preview.refetch()} pending={preview.isFetching} />
            </>
          ) : null}
          {editorPreview && !applyTaskId && dictionariesReady && hasHiddenSelections ? (
            <p className="text-sm text-muted-foreground" role="status">
              部分账号的平台或分组已不在可选字典中，请刷新字典并重新预览后再同步。
            </p>
          ) : null}
          {editorPreview && !applyTaskId && membershipChanged ? (
            <p className="text-sm text-muted-foreground" role="status">
              账号的平台或分组已变化，请关闭后重新选择同步范围。
            </p>
          ) : null}
          {editorPreview && !applyTaskId ? (
            <DictionaryReadState kind="platform">
              <DictionaryReadState kind="group">
                <SyncModelEditor
                  preview={editorPreview}
                  platforms={platforms}
                  scopeExclusions={scopeExclusions}
                  scopeInclusions={scopeInclusions}
                  probeModels={probeModels}
                  probeModelOptions={probeModelOptions}
                  disabled={editorDisabled}
                  onProbeModelsChange={setProbeModels}
                  onModelExcluded={setModelExcluded}
                  onModelBlocked={blocking.block}
                  onAddModel={(ids, model) => {
                    setManualModels((current) => {
                      const next = new Map(current);
                      for (const id of ids) {
                        const models = next.get(id) ?? [];
                        if (
                          ![
                            ...models,
                            ...(preview.data?.accounts.find((account) => account.account_id === id)
                              ?.models ?? []),
                          ].some((value) => value.toLocaleLowerCase() === model.toLocaleLowerCase())
                        ) {
                          next.set(id, [...models, model]);
                        }
                      }
                      return next;
                    });
                  }}
                />
              </DictionaryReadState>
            </DictionaryReadState>
          ) : null}
          {blocking.pending ? (
            <ContentLoading compact label="正在保存全局屏蔽并更新模型列表" />
          ) : null}
          {blocking.refreshFailed ? (
            <ContentRetry onRetry={() => blocking.refresh()} pending={blocking.pending} />
          ) : null}
          {catalogRefreshMessage ? (
            <p
              className="rounded-md border border-primary/30 bg-primary/5 px-3 py-2 text-sm"
              role="status"
            >
              {catalogRefreshMessage}
            </p>
          ) : null}
          {apply.isPending ? <TaskStartupState message="正在创建模型同步任务" /> : null}
          {apply.error && !isModelCatalogChangedError(apply.error) ? (
            <ModelSyncError error={apply.error} fallback="模型应用任务启动失败" />
          ) : null}
          {applyTask.error ? (
            <>
              <ModelSyncError error={applyTask.error} fallback="模型应用状态读取失败" />
              <ContentRetry
                onRetry={() => void applyTask.refetch()}
                pending={applyTask.isFetching}
              />
            </>
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
              disabled={
                includedModelCount === 0 ||
                missingProbeModel ||
                editorDisabled ||
                hasHiddenSelections
              }
              onClick={() => {
                if (submitting.current) return;
                submitting.current = true;
                apply.mutate();
              }}
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
  accountUpstreams: ReadonlyMap<string, ModelSyncUpstream>;
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
    return (
      <p className="break-words text-sm text-destructive" role="alert">
        {props.task.message || "账号模型发现未完成"}
      </p>
    );
  }
  const failed = modelSyncTaskItems(props.task).filter((item) => item.status !== "succeeded");
  if (failed.length === 0) return null;
  const groups = groupModelSyncFailures(failed, props.accountUpstreams);
  return (
    <details
      className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
      aria-live="polite"
    >
      <summary className="cursor-pointer text-destructive">
        <strong>
          {groups.length} 个上游获取模型失败，涉及 {failed.length} 个账号
        </strong>
        <span className="ml-2 text-xs">展开详情</span>
      </summary>
      <div className="mt-2 grid max-h-40 gap-1 overflow-y-auto border-t border-destructive/20 pt-2 text-xs text-destructive">
        {groups.map((group) => {
          return (
            <div className="grid gap-1 break-words" key={group.key}>
              <strong>
                {group.label} · {group.items.length} 个账号
              </strong>
              {group.errors.map((error) => (
                <span key={error.message}>
                  {error.message}
                  <span className="ml-1 opacity-80">
                    （
                    {error.items
                      .map((item) => item.accountName || `账号 #${item.accountId}`)
                      .join("、")}
                    ）
                  </span>
                </span>
              ))}
            </div>
          );
        })}
      </div>
    </details>
  );
}

function SyncModelEditor(props: {
  preview: AccountModelSyncPreview;
  platforms: ModelSyncPlatformGroup[];
  scopeExclusions: ScopeModelExclusions;
  scopeInclusions: ScopeModelExclusions;
  probeModels: string[];
  probeModelOptions: string[];
  disabled: boolean;
  onProbeModelsChange: (models: string[]) => void;
  onModelExcluded: (scopes: string[], model: string, excluded: boolean) => void;
  onModelBlocked: (model: string) => void;
  onAddModel: (ids: string[], model: string) => void;
}) {
  const platforms = props.platforms;
  const [activePlatform, setActivePlatform] = useState(platforms[0]?.key ?? "");
  const [activeGroup, setActiveGroup] = useState(platforms[0]?.groups[0]?.key ?? "");
  const [modelSearch, setModelSearch] = useState("");
  const currentPlatform =
    platforms.find((platform) => platform.key === activePlatform) ?? platforms[0];
  const currentGroup =
    currentPlatform?.groups.find((group) => group.key === activeGroup) ??
    currentPlatform?.groups[0];
  const scopeKeys = currentGroup ? [currentGroup.scopeKey] : [];
  const blockedModels = new Set(
    props.preview.blocked_models.map((model) => model.toLocaleLowerCase()),
  );
  const selectableModels =
    currentGroup?.models.filter(
      (item) =>
        !blockedModels.has(item.model.toLocaleLowerCase()) &&
        !modelMatchesBlockPatterns(item.model, props.preview.blocked_patterns),
    ) ?? [];
  const visibleModels = selectableModels.filter((item) =>
    item.model.toLocaleLowerCase().includes(modelSearch.trim().toLocaleLowerCase()),
  );
  const includedModels = useMemo(
    () =>
      includedGroupModelKeys(
        props.preview.accounts,
        currentGroup,
        currentGroup ? props.scopeExclusions.get(currentGroup.scopeKey) : undefined,
        currentGroup ? props.scopeInclusions.get(currentGroup.scopeKey) : undefined,
      ),
    [props.preview.accounts, currentGroup, props.scopeExclusions, props.scopeInclusions],
  );
  const selectedInGroup = selectableModels.filter((item) =>
    includedModels.has(item.model.toLocaleLowerCase()),
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
    <section className="grid content-start gap-3" aria-labelledby="account-sync-models-title">
      <div
        className="grid self-start gap-3 rounded-lg border p-4"
        data-testid="unified-probe-models"
      >
        <FieldLabel
          label="统一探活模型"
          description="最多选择 20 个；留空时同步完成后不执行探活。"
        />
        <div className="grid gap-2 sm:max-w-2xl">
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
        </div>
      </div>

      <div className="grid content-start gap-3 rounded-lg border p-4">
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
          <div className="grid gap-3" id="account-model-sync-platform-panel" role="tabpanel">
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
              <div className="grid gap-3" id="account-model-sync-group-panel" role="tabpanel">
                <div className="flex items-center justify-between gap-3 text-xs">
                  <span className="text-muted-foreground">
                    已选 {selectedInGroup}/{selectableModels.length} 个模型
                  </span>
                  <span>{currentGroup.accountCount} 个账号</span>
                </div>
                <ManualSyncModelForm
                  key={currentGroup.scopeKey}
                  disabled={props.disabled}
                  blockedPatterns={props.preview.blocked_patterns}
                  onAdd={(model) => {
                    const ids = currentGroup.accountIds;
                    props.onAddModel(ids, model);
                    props.onModelExcluded(scopeKeys, model, false);
                  }}
                />
                <div
                  className="grid min-h-32 max-h-[30rem] auto-rows-[3.5rem] content-start grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
                  data-testid="account-sync-models"
                >
                  {visibleModels.map((item) => {
                    const included = includedModels.has(item.model.toLocaleLowerCase());
                    const showsSupportingAccounts = shouldShowModelSupportingAccounts(
                      item.supportingAccounts.length,
                    );
                    const card = (
                      <div
                        className={cn(
                          "relative flex min-w-0 items-start gap-2 rounded-md border bg-background px-3 py-2.5 transition-colors",
                          props.disabled
                            ? "cursor-not-allowed"
                            : "cursor-pointer hover:border-primary/50 hover:bg-muted/40",
                          included && "border-primary/40",
                        )}
                        key={item.model}
                        role="group"
                        aria-label={`模型 ${item.model}`}
                        aria-disabled={props.disabled}
                        data-model-support-tooltip-trigger={
                          showsSupportingAccounts ? "" : undefined
                        }
                        onClick={() => {
                          if (props.disabled) return;
                          props.onModelExcluded(scopeKeys, item.model, included === true);
                        }}
                      >
                        <Checkbox
                          className="mt-0.5"
                          checked={included === true}
                          disabled={props.disabled}
                          onCheckedChange={(checked) =>
                            props.onModelExcluded(scopeKeys, item.model, checked !== true)
                          }
                          onClick={(event) => event.stopPropagation()}
                          aria-label={`同步模型 ${item.model}`}
                        />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate font-mono text-xs">{item.model}</span>
                          <span className="text-muted-foreground mt-1 block text-xs">
                            {item.accountCount}/{currentGroup.accountCount} 个账号
                          </span>
                        </span>
                      </div>
                    );
                    const contextTrigger = <ContextMenuTrigger render={card} />;
                    const trigger = showsSupportingAccounts ? (
                      <Tooltip>
                        <TooltipTrigger render={contextTrigger} />
                        <TooltipContent
                          role="tooltip"
                          align="start"
                          className="grid max-w-sm gap-1"
                        >
                          <span className="font-medium">支持账号</span>
                          {item.supportingAccounts.map((account) => (
                            <span key={account.accountId}>
                              {account.accountName}
                              {account.platform ? ` · ${account.platform}` : ""}
                            </span>
                          ))}
                        </TooltipContent>
                      </Tooltip>
                    ) : (
                      contextTrigger
                    );
                    return (
                      <ContextMenu key={item.model}>
                        {trigger}
                        <ContextMenuContent>
                          <ContextMenuItem
                            disabled={props.disabled}
                            onClick={() => props.onModelBlocked(item.model)}
                          >
                            <Ban aria-hidden="true" />
                            排除此模型（全局屏蔽）
                          </ContextMenuItem>
                        </ContextMenuContent>
                      </ContextMenu>
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

export function shouldShowModelSupportingAccounts(accountCount: number): boolean {
  return accountCount > 0 && accountCount < 5;
}

export function modelSyncPlatformGroups(
  preview: AccountModelSyncPreview,
  accountPlatforms: ReadonlyMap<string, string | null>,
  accountGroups: ReadonlyMap<string, string[]>,
  dictionaryEntries?: readonly DictionaryEntry[],
  groupDictionaryEntries?: readonly DictionaryEntry[],
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
          accountIds: string[];
          models: Map<string, ModelSyncModel>;
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
    const accountPlatformName = key === "unclassified" ? null : (accountPlatformLabel(key) ?? key);
    const supportingAccount = {
      accountId: account.account_id,
      accountName: account.account_name || `账号 #${account.account_id}`,
      platform: accountPlatformName,
    } satisfies ModelSyncSupportingAccount;
    const memberships = uniqueAccountGroups(accountGroups.get(account.account_id));
    for (const membership of memberships) {
      const groupKey = membership.toLocaleLowerCase();
      let membershipGroup = group.groups.get(groupKey);
      if (!membershipGroup) {
        membershipGroup = { label: membership, accountCount: 0, accountIds: [], models: new Map() };
        group.groups.set(groupKey, membershipGroup);
      }
      membershipGroup.accountCount++;
      membershipGroup.accountIds.push(account.account_id);
      for (const model of account.models) {
        const modelKey = model.toLocaleLowerCase();
        const coverage = membershipGroup.models.get(modelKey);
        if (coverage) {
          coverage.accountCount++;
          coverage.supportingAccounts.push(supportingAccount);
        } else {
          membershipGroup.models.set(modelKey, {
            model,
            accountCount: 1,
            supportingAccounts: [supportingAccount],
          });
        }
      }
    }
  }
  const result = Array.from(groups, ([key, group]) => ({
    key,
    label: group.label,
    accountCount: group.accountCount,
    groups: Array.from(group.groups, ([groupKey, membership]) => ({
      key: groupKey,
      scopeKey: modelSyncScopeKey(key, membership.label),
      label: membership.label,
      accountCount: membership.accountCount,
      accountIds: membership.accountIds,
      models: Array.from(membership.models.values()).sort((left, right) =>
        left.model.localeCompare(right.model),
      ),
    })).sort((left, right) => left.label.localeCompare(right.label)),
  }));
  const ordered = orderedDictionaryOptions(dictionaryEntries);
  for (const platform of result) {
    const ungrouped = platform.groups.filter((group) => group.label === "未分组");
    platform.groups = ungrouped.concat(
      orderByDictionary(
        platform.groups.filter((group) => group.label !== "未分组"),
        groupDictionaryEntries,
        (group) => group.label,
        "name",
      ),
    );
  }
  const registered = ordered.flatMap((entry) => {
    const platform = result.find((item) => item.key === entry.value);
    return platform ? [{ ...platform, label: entry.label }] : [];
  });
  return registered.concat(result.filter((item) => item.key === "unclassified"));
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
  if (props.task.result.probe_disabled === true) resultMessage = "模型写入已完成，未执行探活";
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
          title={props.task.result.probe_disabled === true ? "探活验证（未执行）" : "探活验证"}
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
  return <QueryErrorToast error={props.error} fallback={props.fallback} />;
}
