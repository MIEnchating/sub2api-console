import { useQuery } from "@tanstack/react-query";
import { useMemo, useState, type ReactElement } from "react";

import { api, type TaskSummary } from "@/api";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { RefreshButton } from "@/components/refresh-button";
import { activeTaskStatuses } from "../constants";
import { SystemResources } from "./system-resources";
import { TaskDetailsDialog } from "./task-details-dialog";
import { TaskTable } from "./task-table";
import { TaskToolbar, type TaskListGroup } from "./task-toolbar";

export function SystemInfoPage(): ReactElement {
  const [statusFilter, setStatusFilter] = useState<TaskSummary["status"] | null>(null);
  const [group, setGroup] = useState<TaskListGroup>("active");
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const tasks = useQuery({
    queryKey: ["tasks"],
    queryFn: () => api.tasks(20),
    refetchInterval: (query) =>
      query.state.data?.some((task) => activeTaskStatuses.has(task.status)) ? 1_000 : 10_000,
  });
  const metrics = useQuery({
    queryKey: ["system-metrics"],
    queryFn: api.systemMetrics,
    refetchInterval: 5_000,
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const accountNames = useMemo(
    () => new Map((accounts.data ?? []).map((account) => [account.id, account.name])),
    [accounts.data],
  );
  const groupedTasks = useMemo(() => {
    const active: TaskSummary[] = [];
    const history: TaskSummary[] = [];
    for (const task of tasks.data ?? []) {
      if (task.system_info !== true) continue;
      if (active.length + history.length >= 20) break;
      if (activeTaskStatuses.has(task.status)) active.push(task);
      else history.push(task);
    }
    return { active, history };
  }, [tasks.data]);
  const visibleTasks = groupedTasks[group].filter(
    (task) => statusFilter === null || task.status === statusFilter,
  );

  return (
    <PageLayout fixedContent>
      {tasks.error && <QueryErrorToast error={tasks.error} fallback="任务列表读取失败" />}
      {metrics.error && <QueryErrorToast error={metrics.error} fallback="系统资源统计读取失败" />}
      <PageHeading
        eyebrow="SYSTEM / TASKS"
        title="系统信息"
        description="查看服务器资源占用与后台任务执行情况。"
        action={
          <PageActions>
            <RefreshButton
              pending={tasks.isFetching || metrics.isFetching}
              ariaLabel="刷新系统信息"
              onClick={() => void Promise.all([tasks.refetch(), metrics.refetch()])}
            />
          </PageActions>
        }
      />
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        <SystemResources
          metrics={metrics.data}
          loading={metrics.isLoading}
          failed={metrics.isError}
          refreshing={metrics.isFetching}
          onRetry={() => void metrics.refetch()}
        />
        <DataTablePanel className="flex-1" data-testid="system-tasks-panel">
          <TaskToolbar
            group={group}
            counts={
              tasks.data
                ? { active: groupedTasks.active.length, history: groupedTasks.history.length }
                : null
            }
            statusFilter={statusFilter}
            onGroupChange={(nextGroup) => {
              setGroup(nextGroup);
              setStatusFilter(null);
            }}
            onStatusChange={setStatusFilter}
          />
          <div
            id="system-task-panel"
            role="tabpanel"
            aria-labelledby={`system-task-tab-${group}`}
            className="flex min-h-0 min-w-0 flex-1 flex-col"
          >
            <TaskTable
              tasks={visibleTasks}
              group={group}
              filtered={statusFilter !== null}
              loading={tasks.isLoading}
              unavailable={!tasks.data && tasks.isError}
              refreshing={tasks.isFetching}
              onRetry={() => void tasks.refetch()}
              onClearFilter={() => setStatusFilter(null)}
              onSelect={setSelectedTaskId}
            />
          </div>
        </DataTablePanel>
      </div>
      <TaskDetailsDialog
        taskId={selectedTaskId}
        accountNames={accountNames}
        onClose={() => setSelectedTaskId(null)}
      />
    </PageLayout>
  );
}
