import { automationScheduleLabel } from "../lib/automation-schedule";
import { useEffect, useMemo, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, FolderPlus, ListOrdered, Pencil, Play, Trash2 } from "lucide-react";
import { toast } from "sonner";

import {
  api,
  type AccountModelAutomationSchedule,
  type AccountModelAutomationTaskGroup,
} from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { TaskDetailsDialog } from "@/features/system-info/components/task-details-dialog";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { QueryErrorToast } from "@/components/query-error-toast";
import { RefreshButton } from "@/components/refresh-button";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { notifyOperationError } from "@/lib/operation-feedback";
import { AutomationEditorLoader } from "./automation-editor-loader";
import { formatDateTime } from "@/lib/date-format";
import { AccountModelAutomationTaskGroupDialog } from "./account-model-automation-task-group-dialog";
import type { AutomationTaskGroupForm } from "../lib/automation-task-group";

function scheduleTime(value: string | null): string {
  if (!value) return "未安排";
  return formatDateTime(value);
}

export function AccountModelAutomationPage(): ReactElement {
  const client = useQueryClient();
  const [platformId, setPlatformId] = useState("");
  const [editor, setEditor] = useState<{
    platformId: string;
    schedule?: AccountModelAutomationSchedule;
  } | null>(null);
  const [deleting, setDeleting] = useState<AccountModelAutomationSchedule | null>(null);
  const [taskGroupEditor, setTaskGroupEditor] = useState<AccountModelAutomationTaskGroup | null>(
    null,
  );
  const [taskGroupDialogOpen, setTaskGroupDialogOpen] = useState(false);
  const [deletingTaskGroup, setDeletingTaskGroup] =
    useState<AccountModelAutomationTaskGroup | null>(null);
  const [runningTaskId, setRunningTaskId] = useState<string | null>(null);
  const workspace = useQuery({
    queryKey: ["newapi-workspace", "account-model-automation"],
    queryFn: () => api.newAPIWorkspace(),
    retry: false,
  });
  const schedules = useQuery({
    queryKey: ["account-model-automation-schedules"],
    queryFn: ({ signal }) => api.accountModelAutomationSchedules(signal),
    retry: false,
  });
  const taskGroups = useQuery({
    queryKey: ["account-model-automation-task-groups"],
    queryFn: ({ signal }) => api.accountModelAutomationTaskGroups(signal),
    retry: false,
  });

  useEffect(() => {
    if (!platformId && workspace.data?.platforms[0]) {
      setPlatformId(workspace.data.platforms[0].id);
    }
  }, [platformId, workspace.data?.platforms]);

  const platforms = workspace.data?.platforms ?? [];
  const platformNames = useMemo(
    () => new Map(platforms.map((platform) => [platform.id, platform.name])),
    [platforms],
  );
  const selectedPlatform = platforms.find((platform) => platform.id === platformId);
  const scheduleNames = useMemo(
    () => new Map((schedules.data ?? []).map((schedule) => [schedule.id, schedule.name])),
    [schedules.data],
  );
  const deleteMutation = useMutation({
    mutationFn: (schedule: AccountModelAutomationSchedule) =>
      api.deleteAccountModelAutomationSchedule(schedule.id),
    onSuccess: async () => {
      setDeleting(null);
      await Promise.all([
        client.invalidateQueries({ queryKey: ["account-model-automation-schedules"] }),
        client.invalidateQueries({ queryKey: ["account-model-automation-task-groups"] }),
      ]);
      toast.success("运营任务已删除");
    },
    onError: (error) => notifyOperationError(error, "运营任务删除失败"),
  });
  const runMutation = useMutation({
    mutationFn: (schedule: AccountModelAutomationSchedule) =>
      api.runAccountModelAutomationSchedule(schedule.id),
    onSuccess: async (task) => {
      setRunningTaskId(task.id);
      toast.success("运营任务已启动");
      await client.invalidateQueries({ queryKey: ["account-model-automation-schedules"] });
    },
    onError: (error) => notifyOperationError(error, "运营任务启动失败"),
  });
  const saveTaskGroupMutation = useMutation({
    mutationFn: (input: AutomationTaskGroupForm) =>
      api.saveAccountModelAutomationTaskGroup(
        taskGroupEditor ? { ...input, id: taskGroupEditor.id } : input,
      ),
    onSuccess: async () => {
      setTaskGroupDialogOpen(false);
      setTaskGroupEditor(null);
      await client.invalidateQueries({ queryKey: ["account-model-automation-task-groups"] });
      toast.success("运营任务组已保存");
    },
    onError: (error) => notifyOperationError(error, "运营任务组保存失败"),
  });
  const deleteTaskGroupMutation = useMutation({
    mutationFn: (group: AccountModelAutomationTaskGroup) =>
      api.deleteAccountModelAutomationTaskGroup(group.id),
    onSuccess: async () => {
      setDeletingTaskGroup(null);
      await client.invalidateQueries({ queryKey: ["account-model-automation-task-groups"] });
      toast.success("运营任务组已删除");
    },
    onError: (error) => notifyOperationError(error, "运营任务组删除失败"),
  });
  const runTaskGroupMutation = useMutation({
    mutationFn: (group: AccountModelAutomationTaskGroup) =>
      api.runAccountModelAutomationTaskGroup(group.id),
    onSuccess: (task) => {
      setRunningTaskId(task.id);
      toast.success("运营任务组已启动");
    },
    onError: (error) => notifyOperationError(error, "运营任务组启动失败"),
  });
  const openNew = (): void => {
    if (selectedPlatform) setEditor({ platformId: selectedPlatform.id });
  };
  const openEdit = (schedule: AccountModelAutomationSchedule): void => {
    setEditor({ platformId: schedule.request.platform_id, schedule });
  };

  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="运营管理"
        title="账号模型"
        description="管理账号模型同步任务，按需手动运行或设置定时运行。"
        action={
          <PageActions>
            {platforms.length > 1 && (
              <Select value={platformId} onValueChange={(value) => setPlatformId(value ?? "")}>
                <SelectTrigger aria-label="选择 New API 平台" className="w-56">
                  <SelectValue placeholder="选择 New API 平台" />
                </SelectTrigger>
                <SelectContent>
                  {platforms.map((platform) => (
                    <SelectItem key={platform.id} value={platform.id}>
                      {platform.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <RefreshButton
              pending={schedules.isFetching || taskGroups.isFetching}
              onClick={() => {
                void schedules.refetch();
                void taskGroups.refetch();
              }}
              ariaLabel="刷新运营任务列表"
            />
            <Button
              variant="outline"
              disabled={(schedules.data?.length ?? 0) < 2}
              onClick={() => {
                setTaskGroupEditor(null);
                setTaskGroupDialogOpen(true);
              }}
            >
              <FolderPlus aria-hidden="true" />
              添加任务组
            </Button>
            <Button disabled={!selectedPlatform} onClick={openNew}>
              <CirclePlus aria-hidden="true" />
              添加任务
            </Button>
          </PageActions>
        }
      />
      {workspace.error && (
        <QueryErrorToast error={workspace.error} fallback="New API 平台读取失败" />
      )}
      {schedules.error && <QueryErrorToast error={schedules.error} fallback="运营任务读取失败" />}
      {taskGroups.error && (
        <QueryErrorToast error={taskGroups.error} fallback="运营任务组读取失败" />
      )}
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        {workspace.data && platforms.length === 0 && (
          <div className="text-muted-foreground flex min-h-72 items-center justify-center rounded-md border border-dashed bg-background px-6 text-center text-sm">
            尚未添加 New API 平台，请先在系统设置中完成配置。
          </div>
        )}
        {workspace.isPending && <PageLoadingSkeleton label="正在读取 New API 平台" fill />}
        {!workspace.data && workspace.isError && (
          <ContentRetry pending={workspace.isFetching} onRetry={() => void workspace.refetch()} />
        )}
        {schedules.isPending && (
          <PageLoadingSkeleton label="正在读取运营任务" variant="table" fill />
        )}
        {!schedules.data && schedules.isError && (
          <ContentRetry pending={schedules.isFetching} onRetry={() => void schedules.refetch()} />
        )}
        {schedules.data && (
          <>
            <section
              aria-label="运营任务组"
              className="grid gap-3 rounded-lg border bg-muted/20 p-4"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <ListOrdered aria-hidden="true" />
                  运营任务组
                </div>
                <span className="text-muted-foreground text-xs">按成员顺序依次执行</span>
              </div>
              {taskGroups.isPending && (
                <PageLoadingSkeleton label="正在读取任务组" variant="list" framed={false} />
              )}
              {!taskGroups.data && taskGroups.isError && (
                <ContentRetry
                  pending={taskGroups.isFetching}
                  onRetry={() => void taskGroups.refetch()}
                />
              )}
              {taskGroups.data?.length === 0 && !taskGroups.isPending && (
                <p className="text-muted-foreground text-sm">
                  选择两个或更多运营任务后，可创建任务组连续执行。
                </p>
              )}
              {taskGroups.data && taskGroups.data.length > 0 && (
                <div role="list" className="grid gap-2 lg:grid-cols-2">
                  {taskGroups.data.map((group) => {
                    const names = group.schedule_ids
                      .map((id) => scheduleNames.get(id))
                      .filter((name): name is string => Boolean(name));
                    return (
                      <div
                        key={group.id}
                        role="listitem"
                        className="grid min-w-0 gap-2 rounded-md border bg-background p-3"
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="min-w-0 flex-1 truncate text-sm font-medium">
                            {group.name}
                          </span>
                          <span className="text-muted-foreground shrink-0 text-xs">
                            {group.schedule_ids.length} 个任务
                          </span>
                        </div>
                        <p className="text-muted-foreground min-w-0 truncate text-xs">
                          {names.join(" → ") || "任务已变更，请编辑任务组"}
                        </p>
                        <div
                          role="group"
                          aria-label={`${group.name} 的操作`}
                          className="flex justify-end gap-1"
                        >
                          <TableActionButton
                            label="运行任务组"
                            disabled={runTaskGroupMutation.isPending}
                            onClick={() => runTaskGroupMutation.mutate(group)}
                          >
                            <Play aria-hidden="true" />
                          </TableActionButton>
                          <TableActionButton
                            label="编辑任务组"
                            onClick={() => {
                              setTaskGroupEditor(group);
                              setTaskGroupDialogOpen(true);
                            }}
                          >
                            <Pencil aria-hidden="true" />
                          </TableActionButton>
                          <TableActionButton
                            label="删除任务组"
                            tone="danger"
                            disabled={deleteTaskGroupMutation.isPending}
                            onClick={() => setDeletingTaskGroup(group)}
                          >
                            <Trash2 aria-hidden="true" />
                          </TableActionButton>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </section>
            <DataTablePanel className="min-h-0 flex-1">
              <Table
                aria-label="账号模型任务"
                actionColumn
                className="min-w-[60rem]"
                containerClassName="min-h-0 flex-1 overflow-auto"
              >
                <TableHeader>
                  <TableRow>
                    <TableHead>任务名称</TableHead>
                    <TableHead>平台</TableHead>
                    <TableHead>执行范围</TableHead>
                    <TableHead>运行方式</TableHead>
                    <TableHead>下次运行</TableHead>
                    <TableHead className="w-32 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {schedules.data.length === 0 && (
                    <TableEmptyState columns={6}>
                      暂无运营任务，点击右上角“添加任务”创建。
                    </TableEmptyState>
                  )}
                  {schedules.data.map((schedule) => (
                    <TableRow key={schedule.id}>
                      <TableCell>
                        <div className="grid min-w-40 gap-1">
                          <span className="font-medium">{schedule.name}</span>
                          <span className="text-muted-foreground text-xs">
                            {schedule.enabled ? "定时运行已开启" : "手动运行"}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        {platformNames.get(schedule.request.platform_id) ??
                          schedule.request.platform_id}
                      </TableCell>
                      <TableCell>
                        {schedule.request.group_ids.length} 个分组 ·{" "}
                        {schedule.request.channels.length} 个渠道
                      </TableCell>
                      <TableCell>
                        {schedule.enabled ? (
                          <span className="text-success">{automationScheduleLabel(schedule)}</span>
                        ) : (
                          <span className="text-muted-foreground">仅手动运行</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="grid gap-1 text-sm">
                          <span>
                            {schedule.enabled ? scheduleTime(schedule.next_run_at) : "未启用"}
                          </span>
                          <span className="text-muted-foreground text-xs">
                            上次：{scheduleTime(schedule.last_run_at)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell overflowTooltip={false}>
                        <div
                          role="group"
                          aria-label={`${schedule.name} 的操作`}
                          className="flex justify-end gap-1"
                        >
                          <TableActionButton
                            label="运行"
                            disabled={runMutation.isPending}
                            onClick={() => runMutation.mutate(schedule)}
                          >
                            <Play aria-hidden="true" />
                          </TableActionButton>
                          <TableActionButton label="编辑" onClick={() => openEdit(schedule)}>
                            <Pencil aria-hidden="true" />
                          </TableActionButton>
                          <TableActionButton
                            label="删除"
                            tone="danger"
                            disabled={deleteMutation.isPending}
                            onClick={() => setDeleting(schedule)}
                          >
                            <Trash2 aria-hidden="true" />
                          </TableActionButton>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </DataTablePanel>
          </>
        )}
      </div>
      {editor && (
        <AutomationEditorLoader
          platformId={editor.platformId}
          schedule={editor.schedule}
          onOpenChange={(open) => {
            if (!open) setEditor((current) => (current === editor ? null : current));
          }}
        />
      )}
      <ConfirmActionDialog
        open={deleting !== null}
        title="删除运营任务"
        description={deleting ? `确定删除任务“${deleting.name}”吗？` : ""}
        confirmLabel="删除"
        pending={deleteMutation.isPending}
        onOpenChange={(open) => {
          if (!open && !deleteMutation.isPending) setDeleting(null);
        }}
        onConfirm={() => {
          if (deleting) deleteMutation.mutate(deleting);
        }}
      />
      <ConfirmActionDialog
        open={deletingTaskGroup !== null}
        title="删除运营任务组"
        description={deletingTaskGroup ? `确定删除任务组“${deletingTaskGroup.name}”吗？` : ""}
        confirmLabel="删除"
        pending={deleteTaskGroupMutation.isPending}
        onOpenChange={(open) => {
          if (!open && !deleteTaskGroupMutation.isPending) setDeletingTaskGroup(null);
        }}
        onConfirm={() => {
          if (deletingTaskGroup) deleteTaskGroupMutation.mutate(deletingTaskGroup);
        }}
      />
      {taskGroupDialogOpen && schedules.data && (
        <AccountModelAutomationTaskGroupDialog
          open
          group={taskGroupEditor ?? undefined}
          schedules={schedules.data}
          pending={saveTaskGroupMutation.isPending}
          onOpenChange={(open) => {
            if (!open && !saveTaskGroupMutation.isPending) {
              setTaskGroupDialogOpen(false);
              setTaskGroupEditor(null);
            }
          }}
          onSave={(input) => saveTaskGroupMutation.mutate(input)}
        />
      )}
      <TaskDetailsDialog taskId={runningTaskId} onClose={() => setRunningTaskId(null)} />
    </PageLayout>
  );
}
