import { useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  automationScheduleNameSchema,
  automationTimingSchema,
  type AutomationTimingForm,
} from "../lib/automation-schedule";
import type { AutomationChannelPageControls } from "../hooks/use-automation-channel-catalog";
import { resolveAutomationChannels } from "../lib/automation-channel-selection";
import { AutomationChannelSelector } from "./automation-channel-selector";
import { AutomationScheduleTiming } from "./automation-schedule-timing";
import { useAutomationPreviewLifecycle } from "../hooks/use-automation-preview-lifecycle";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Play, Save, Search } from "lucide-react";
import { toast } from "sonner";

import {
  api,
  type AccountModelAutomationSchedule,
  type AccountModelAutomationScheduleInput,
  type GroupMembershipOption,
  type NewAPIChannel,
  type Task,
} from "@/api";
import { MultiSelect } from "@/components/multi-select";
import {
  AccountModelAutomationResults,
  automationModels,
} from "./account-model-automation-results";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { QueryErrorToast } from "@/components/query-error-toast";
import { cn } from "@/lib/utils";

function finished(task: Task | null | undefined): boolean {
  return !!task && ["succeeded", "partial", "failed", "cancelled"].includes(task.status);
}

export function AccountModelAutomationDialog(props: {
  platformId: string;
  open: boolean;
  embedded?: boolean;
  withinDialog?: boolean;
  schedule?: AccountModelAutomationSchedule;
  onOpenChange: (open: boolean) => void;
  channels: NewAPIChannel[];
  channelIDs?: string[];
  onChannelIDsChange?: (ids: string[]) => void;
  channelPageControls?: AutomationChannelPageControls;
  channelsReady?: boolean;
  channelsPending?: boolean;
  onRefreshChannels?: () => void;
}): ReactElement {
  const client = useQueryClient();
  const groups = useQuery({
    queryKey: ["groups", "memberships"],
    queryFn: ({ signal }) => api.groupMembershipOptions(signal),
    enabled: props.open,
  });
  const workspace = useQuery({
    queryKey: ["newapi-workspace", props.platformId],
    queryFn: ({ signal }) => api.newAPIWorkspace(props.platformId, signal),
    enabled: props.open,
  });
  const handledTask = useRef("");
  const nameInput = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!props.open || !props.withinDialog) return;
    // The dialog restores focus after removing its loading/retry controls.
    // Hand off to the form after that frame, without moving an existing user focus.
    const frame = requestAnimationFrame(() => {
      const input = nameInput.current;
      if (!input) return;
      const active = input.ownerDocument.activeElement;
      if (active === input.ownerDocument.body || active === input.closest('[role="dialog"]')) {
        input.focus({ preventScroll: true });
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [props.open, props.withinDialog]);
  const revision = useRef(0);
  const [selectedModels, setSelectedModels] = useState<string[]>([]);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [discovery, setDiscovery] = useState(false);
  const [groupIDs, setGroupIDs] = useState<string[]>([]);
  const [localChannelIDs, setLocalChannelIDs] = useState<string[]>([]);
  const channelIDs = props.channelIDs ?? localChannelIDs;
  const setChannelIDs = props.onChannelIDsChange ?? setLocalChannelIDs;
  const channelSelection = useMemo(
    () => resolveAutomationChannels(props.channels, channelIDs),
    [props.channels, channelIDs],
  );
  const channelsReady = props.channelsReady !== false && channelSelection.targets !== null;
  const requireChannelTargets = (): Array<{ id: string; version: string }> => {
    if (!channelsReady || channelSelection.targets === null) {
      throw new Error("渠道范围尚未确认，请重新读取或调整选择后重试");
    }
    return channelSelection.targets;
  };
  const [publishMissing, setPublishMissing] = useState(true);
  const [priceEnabled, setPriceEnabled] = useState(true);
  const [scheduleName, setScheduleName] = useState("");
  const timing = useForm<AutomationTimingForm>({
    resolver: zodResolver(automationTimingSchema),
    mode: "onChange",
    defaultValues: { schedule_type: "interval", interval_minutes: "60", daily_times: ["09:00"] },
  });
  const timingValue = timing.watch();
  const intervalMinutes = timingValue.interval_minutes;
  const scheduleType = timingValue.schedule_type;
  const dailyTimes = timingValue.daily_times;
  const [scheduleEnabled, setScheduleEnabled] = useState(true);
  const [task, setTask] = useState<Task | null>(null);
  const [taskChannelScope, setTaskChannelScope] = useState<string | null>(null);
  const canSyncPrices = channelIDs.length > 0;
  useEffect(() => {
    revision.current += 1;
    if (!props.open) return;
    const request = props.schedule?.request;
    setGroupIDs(request?.group_ids ?? []);
    setSelectedModels(request?.selected_models ?? []);
    setAvailableModels([]);
    setDiscovery(false);
    setLocalChannelIDs(request?.channels.map((channel) => channel.id) ?? []);
    setPublishMissing(request?.publish_missing ?? true);
    setPriceEnabled(request?.price_enabled ?? true);
    setScheduleName(props.schedule?.name ?? "");
    timing.reset({
      interval_minutes: String(props.schedule?.interval_minutes || 60),
      schedule_type: props.schedule?.schedule_type ?? "interval",
      daily_times: props.schedule?.daily_times ?? ["09:00"],
    });
    setScheduleEnabled(props.schedule?.enabled ?? true);
    setTask(null);
    setTaskChannelScope(null);
    handledTask.current = "";
  }, [props.open, props.schedule, timing]);
  const taskQuery = useQuery({
    queryKey: ["tasks", task?.id],
    queryFn: ({ signal }) => api.task(task!.id, signal),
    enabled: props.open && !!task,
    retry: false,
    refetchInterval: (query) => (query.state.error || finished(query.state.data) ? false : 1000),
  });
  const currentTask = taskQuery.data ?? task;
  const previewLifecycle = useAutomationPreviewLifecycle(props.open, currentTask);
  const handleOpenChange = (open: boolean): void => {
    if (!open) {
      revision.current += 1;
      previewLifecycle.close();
    }
    props.onOpenChange(open);
  };
  const boundGroupIDs = new Set(
    (workspace.data?.bindings ?? []).map((binding) => binding.sub2api_group_id),
  );
  const boundGroups = (groups.data ?? []).filter(
    (group) => group.id !== null && boundGroupIDs.has(group.id),
  );
  const selectedGroups = groupIDs.map((id) => boundGroups.find((group) => group.id === id));
  const groupsReady =
    !groups.isPending &&
    !workspace.isPending &&
    !groups.isFetching &&
    !workspace.isFetching &&
    !groups.isError &&
    !workspace.isError &&
    selectedGroups.every((group) => group !== undefined);
  const groupScope = JSON.stringify(
    selectedGroups.map((group, index) => [groupIDs[index], group?.account_count ?? null]),
  );
  const currentScope = JSON.stringify([channelSelection.signature, groupScope]);
  const outdatedPreview =
    !discovery &&
    currentTask?.result.dry_run === true &&
    taskChannelScope !== null &&
    taskChannelScope !== currentScope;
  const readyPreview =
    !discovery &&
    !outdatedPreview &&
    channelsReady &&
    groupsReady &&
    currentTask?.result.dry_run === true &&
    finished(currentTask)
      ? currentTask
      : null;
  useEffect(() => {
    if (discovery && currentTask?.status === "succeeded") {
      setAvailableModels(automationModels(currentTask.result.models));
    }
  }, [discovery, currentTask]);
  useEffect(() => {
    if (
      !currentTask ||
      !finished(currentTask) ||
      currentTask.result.dry_run === true ||
      handledTask.current === currentTask.id
    )
      return;
    handledTask.current = currentTask.id;
    void client.invalidateQueries({ queryKey: ["newapi-channels", props.platformId] });
    void client.invalidateQueries({
      queryKey: ["newapi-management-model-prices", props.platformId],
    });
    void client.invalidateQueries({ queryKey: ["newapi-remote-snapshot", props.platformId] });
  }, [client, currentTask, props.platformId]);
  const mutation = useMutation({
    mutationFn: async (input: {
      previewTaskId?: string;
      discovery?: boolean;
      revision: number;
    }) => {
      if (input.previewTaskId && outdatedPreview) {
        throw new Error("渠道范围或配置已变化，请重新预览后再确认写入");
      }
      const channelScope = currentScope;
      const channels = input.discovery ? [] : requireChannelTargets();
      const accept = previewLifecycle.begin(!input.previewTaskId);
      const created = await api.enqueueAccountModelAutomation({
        group_ids: groupIDs,
        platform_id: props.platformId,
        channels,
        publish_missing: !input.discovery && canSyncPrices && publishMissing,
        selected_models: input.discovery ? [] : selectedModels,
        price_enabled: !input.discovery && canSyncPrices && publishMissing && priceEnabled,
        pricing_source: "official",
        dry_run: !input.previewTaskId,
        preview_task_id: input.previewTaskId,
      });
      return { created, visible: accept(created), channelScope };
    },
    onSuccess: (response, input) => {
      if (!response.visible || input.revision !== revision.current) return;
      setDiscovery(input.discovery === true);
      setTask(response.created);
      setTaskChannelScope(input.discovery ? null : response.channelScope);
      if (input.previewTaskId) {
        toast.success("账号模型任务已创建");
      }
    },
    onError: (error) => notifyOperationError(error, "账号模型任务创建失败"),
  });
  const scheduleMutation = useMutation({
    mutationFn: () => {
      const payload: AccountModelAutomationScheduleInput = {
        name: scheduleName.trim(),
        group_ids: groupIDs,
        platform_id: props.platformId,
        channels: requireChannelTargets(),
        publish_missing: canSyncPrices && publishMissing,
        selected_models: selectedModels,
        price_enabled: canSyncPrices && publishMissing && priceEnabled,
        pricing_source: "official",
        enabled: scheduleEnabled,
        interval_minutes: scheduleType === "daily" ? 60 : Number(intervalMinutes),
        ...(scheduleType === "daily"
          ? { schedule_type: "daily" as const, daily_times: dailyTimes, timezone: "Asia/Shanghai" }
          : {}),
      };
      return api.saveAccountModelAutomationSchedule(
        props.schedule ? { ...payload, id: props.schedule.id } : payload,
      );
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["account-model-automation-schedules"] });
      toast.success("运营任务已保存");
      props.onOpenChange(false);
    },
    onError: (error) => notifyOperationError(error, "运营任务保存失败"),
  });
  const invalidatePreview = () => {
    revision.current += 1;
    setDiscovery(false);
    setTask(null);
  };
  const toggle = (values: string[], value: string, setter: (next: string[]) => void) => {
    setter(values.includes(value) ? values.filter((item) => item !== value) : [...values, value]);
    invalidatePreview();
  };
  const validScheduleTiming = automationTimingSchema.safeParse(timingValue).success;
  const nameValidation = automationScheduleNameSchema.safeParse(scheduleName);
  const nameError = nameValidation.success ? null : nameValidation.error.issues[0]?.message;
  const busy =
    mutation.isPending || scheduleMutation.isPending || (!!currentTask && !finished(currentTask));
  const selectionReady =
    channelsReady && (!canSyncPrices || !publishMissing || selectedModels.length > 0);
  const canSaveSchedule =
    selectionReady &&
    groupsReady &&
    !busy &&
    nameValidation.success &&
    validScheduleTiming &&
    groupIDs.length > 0 &&
    !groups.isPending &&
    !workspace.isPending &&
    !groups.isError &&
    !workspace.isError &&
    !scheduleMutation.isPending;

  const content = (
    <>
      {props.embedded ? (
        <div className="border-b px-4 py-4">
          <h1 className="text-lg font-semibold">账号模型</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            选择账号分组，按需选择目标渠道，预览影响范围后确认执行。
          </p>
        </div>
      ) : (
        <DialogHeader>
          <DialogTitle>{props.schedule ? "编辑运营任务" : "添加运营任务"}</DialogTitle>
          <DialogDescription>
            配置账号分组，按需选择目标渠道。保存任务后可手动或定时运行。
          </DialogDescription>
        </DialogHeader>
      )}
      <DialogBody
        className={cn("grid content-start gap-5 overflow-y-auto", props.embedded && "flex-1")}
      >
        {groups.isLoading || workspace.isLoading ? (
          <ContentLoading label="正在读取分组绑定" />
        ) : null}
        {groups.isError && <QueryErrorToast error={groups.error} fallback="分组列表读取失败" />}
        {workspace.isError && (
          <QueryErrorToast error={workspace.error} fallback="分组绑定读取失败" />
        )}
        {groups.isError && (
          <ContentRetry pending={groups.isFetching} onRetry={() => void groups.refetch()} />
        )}
        {workspace.isError && (
          <ContentRetry pending={workspace.isFetching} onRetry={() => void workspace.refetch()} />
        )}
        <section
          className="grid items-start gap-4 border-b pb-5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]"
          aria-label="任务设置"
        >
          <label className="grid content-start gap-1.5 text-sm font-medium">
            任务名称
            <Input
              aria-label="运营任务名称"
              ref={nameInput}
              placeholder="例如：生产组同步"
              value={scheduleName}
              disabled={scheduleMutation.isPending}
              aria-invalid={Boolean(scheduleName && nameError)}
              onChange={(event) => setScheduleName(event.target.value)}
            />
            {scheduleName && nameError ? (
              <span className="text-destructive text-xs">{nameError}</span>
            ) : null}
          </label>
          <div className="grid content-start gap-3">
            <label className="flex items-center gap-2 text-sm font-medium">
              <Checkbox
                checked={scheduleEnabled}
                disabled={scheduleMutation.isPending}
                onCheckedChange={(value) => setScheduleEnabled(value === true)}
              />
              启用定时运行
            </label>
            <AutomationScheduleTiming form={timing} pending={busy || scheduleMutation.isPending} />
          </div>
        </section>
        <section className="grid content-start gap-3" aria-label="执行范围">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2 text-sm font-medium">执行范围</div>
            <span className="text-muted-foreground text-xs">
              已选择 {groupIDs.length} 个分组 · {channelIDs.length} 个渠道
            </span>
          </div>
          <div className="grid items-start gap-4 sm:grid-cols-2">
            <fieldset className="grid min-w-0 content-start gap-2" disabled={busy}>
              <legend className="text-sm font-medium">账号分组</legend>
              <div className="grid h-40 content-start gap-1 overflow-auto overscroll-contain rounded-md border bg-muted/20 p-1">
                {boundGroups.map((group: GroupMembershipOption) => (
                  <label
                    key={group.id ?? group.name}
                    className="flex min-h-9 items-center gap-2 rounded px-2 text-sm hover:bg-background has-[[data-checked]]:bg-primary/10"
                  >
                    <Checkbox
                      checked={group.id !== null && groupIDs.includes(group.id)}
                      disabled={busy || group.id === null}
                      onCheckedChange={() => {
                        if (!group.id) return;
                        toggle(groupIDs, group.id, setGroupIDs);
                        setAvailableModels([]);
                        setSelectedModels([]);
                      }}
                    />
                    <span className="min-w-0 truncate">
                      {group.name}（{group.account_count} 个账号）
                    </span>
                  </label>
                ))}
                {!groups.isLoading && !workspace.isLoading && boundGroups.length === 0 && (
                  <p className="text-muted-foreground p-3 text-sm">当前平台尚未绑定本地分组</p>
                )}
              </div>
            </fieldset>
            <AutomationChannelSelector
              selection={channelSelection}
              pageControls={props.channelPageControls}
              disabled={busy || scheduleMutation.isPending}
              pending={props.channelsPending}
              onRefresh={props.onRefreshChannels}
              onToggle={(id) => toggle(channelIDs, id, setChannelIDs)}
            />
          </div>
        </section>
        <div className="grid min-w-0 items-start gap-5 border-t pt-4 sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
          <section aria-label="上架模型选择" className="grid min-w-0 content-start gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-medium">选择要上架的模型</h3>
              <Button
                variant="outline"
                disabled={
                  busy ||
                  groupIDs.length === 0 ||
                  groups.isPending ||
                  workspace.isPending ||
                  groups.isError ||
                  workspace.isError
                }
                onClick={() => {
                  invalidatePreview();
                  mutation.mutate({ discovery: true, revision: revision.current });
                }}
              >
                <Search aria-hidden="true" />
                读取可用模型
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              读取所选分组的模型后，选择需要上架的模型。
            </p>
            <MultiSelect
              title="请选择模型"
              ariaLabel="要上架的模型"
              options={availableModels.map((model) => ({ value: model, label: model }))}
              selected={selectedModels}
              disabled={busy || (availableModels.length === 0 && selectedModels.length === 0)}
              onChange={(values) => {
                setSelectedModels(values);
                invalidatePreview();
              }}
              maxVisibleChips={6}
              clearText="清空模型选择"
              emptyText="请先读取可用模型"
            />
            <p className="text-xs text-muted-foreground">
              已读取 {availableModels.length} 个可用模型 · 已选择 {selectedModels.length} 个上架模型
            </p>
            {canSyncPrices && publishMissing && selectedModels.length === 0 && (
              <p className="text-xs text-muted-foreground">
                至少选择一个模型后，才能预览或保存上架任务。
              </p>
            )}
          </section>
          <section className="grid content-start gap-3 sm:border-l sm:pl-5" aria-label="同步策略">
            <div className="flex items-center gap-2 text-sm font-medium">同步策略</div>
            <div className="grid content-start gap-3">
              <label className="flex min-h-8 items-center gap-2 text-sm has-disabled:text-muted-foreground">
                <Checkbox
                  checked={canSyncPrices && publishMissing}
                  disabled={!canSyncPrices || busy}
                  onCheckedChange={(value) => {
                    setPublishMissing(value === true);
                    invalidatePreview();
                  }}
                />
                上架所选模型
              </label>
              <label className="flex min-h-8 items-center gap-2 text-sm has-disabled:text-muted-foreground">
                <Checkbox
                  checked={canSyncPrices && publishMissing && priceEnabled}
                  disabled={!canSyncPrices || !publishMissing || busy}
                  onCheckedChange={(value) => {
                    setPriceEnabled(value === true);
                    invalidatePreview();
                  }}
                />
                自动同步模型价格
              </label>
            </div>
          </section>
        </div>
        {taskQuery.isError && (
          <ContentRetry pending={taskQuery.isFetching} onRetry={() => void taskQuery.refetch()} />
        )}
        {currentTask && !finished(currentTask) && currentTask.progress === 0 && (
          <TaskStartupState message={currentTask.message} />
        )}
        {currentTask && !finished(currentTask) && currentTask.progress > 0 && (
          <TaskProgressState message={currentTask.message} progress={currentTask.progress} />
        )}
        {currentTask && !finished(currentTask) && (
          <p className="text-muted-foreground text-xs">
            {previewLifecycle.ownsCurrentTask
              ? "关闭弹窗将取消本次读取或预览任务。"
              : "关闭弹窗后，正式执行任务会继续在后台运行，可在任务记录中查看。"}
          </p>
        )}
        {outdatedPreview && (
          <p className="text-muted-foreground text-xs">
            渠道或分组范围已变化，请重新预览后再确认写入。
          </p>
        )}
        {!groups.isPending &&
          !workspace.isPending &&
          !groupsReady &&
          selectedGroups.some((group) => group === undefined) && (
            <p className="text-muted-foreground text-xs">
              所选分组已不在当前平台绑定中，请重新选择分组。
            </p>
          )}
        {currentTask && finished(currentTask) && (
          <AccountModelAutomationResults task={currentTask} />
        )}
      </DialogBody>
      <DialogFooter className="sm:items-center">
        <Button variant="outline" onClick={() => handleOpenChange(false)}>
          {currentTask && !finished(currentTask) && !previewLifecycle.ownsCurrentTask
            ? "关闭弹窗"
            : "取消"}
        </Button>
        <Button
          variant="outline"
          disabled={!canSaveSchedule}
          onClick={timing.handleSubmit(() => scheduleMutation.mutate())}
        >
          <Save aria-hidden="true" />
          {scheduleMutation.isPending ? "保存中" : "保存任务"}
        </Button>
        <Button
          disabled={!selectionReady || groupIDs.length === 0 || !groupsReady || busy}
          onClick={() => mutation.mutate({ revision: revision.current })}
        >
          <Search aria-hidden="true" />
          预览
        </Button>
        {readyPreview?.status === "succeeded" && currentTask?.id === readyPreview.id && (
          <Button
            disabled={busy}
            onClick={() => {
              mutation.mutate({ previewTaskId: readyPreview.id, revision: revision.current });
            }}
          >
            <Play aria-hidden="true" />
            确认写入
          </Button>
        )}
      </DialogFooter>
    </>
  );
  if (props.withinDialog) return content;
  if (props.embedded) {
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-background">
        {content}
      </div>
    );
  }
  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent width="wide" height="adaptive">
        {content}
      </DialogContent>
    </Dialog>
  );
}
