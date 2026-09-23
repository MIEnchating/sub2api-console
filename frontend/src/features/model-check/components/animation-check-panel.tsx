import { zodResolver } from "@hookform/resolvers/zod";
import { Tabs } from "@base-ui/react/tabs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { lazy, Suspense, useCallback, useEffect, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { api, type AccountStatus, type AnimationTarget, type PrecheckQuestionID } from "@/api";
import { allPrecheckQuestions } from "../constants";
import { useAnimationTasks } from "../hooks/use-animation-tasks";
import { animationSchema, type AnimationForm } from "../lib/animation-schema";
import { AnimationSelection } from "./animation-selection";
import { RefreshCw, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AnimationScheduleDialog, type ScheduleTarget } from "./animation-schedule-dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { CustomAnimationPanel } from "./custom-animation-panel";
import {
  AccountManualPriorityDialog,
  type ManualPriorityValues,
} from "@/features/accounts/components/manual-priority-dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskPollInterval } from "@/lib/task-state";
import { toast } from "sonner";
import { ContentLoading } from "@/components/content-loading";
import { defaultAnimationFilters } from "../lib/animation-filters";
import { DetectionAccountControlProvider } from "./detection-account-control-provider";

const TerminalContinuityPanel = lazy(() =>
  import("./terminal-continuity-panel").then((module) => ({
    default: module.TerminalContinuityPanel,
  })),
);
const DetectionTaskPanel = lazy(() =>
  import("./detection-task-panel").then((module) => ({ default: module.DetectionTaskPanel })),
);

export function AnimationCheckPanel(props: {
  active?: boolean;
  accountID?: string;
  showAllAccounts?: boolean;
}): ReactElement {
  return (
    <DetectionAccountControlProvider>
      <AnimationCheckContent {...props} />
    </DetectionAccountControlProvider>
  );
}

function AnimationCheckContent(props: {
  active?: boolean;
  accountID?: string;
  showAllAccounts?: boolean;
}): ReactElement {
  const [tab, setTab] = useState("accounts");
  const [filters, setFilters] = useState(defaultAnimationFilters);
  const tasks = useAnimationTasks(props.active);
  const [scheduleEdit, setScheduleEdit] = useState<{
    mode: "animation" | "precheck";
    targets: ScheduleTarget[];
    batch: boolean;
  } | null>(null);
  const [manualPriorityAccount, setManualPriorityAccount] = useState<AccountStatus | null>(null);
  const [manualPriorityTaskID, setManualPriorityTaskID] = useState<string | null>(null);
  const client = useQueryClient();
  const [precheckQuestions, setPrecheckQuestions] =
    useState<PrecheckQuestionID[]>(allPrecheckQuestions);
  const form = useForm<AnimationForm>({
    resolver: zodResolver(animationSchema),
    defaultValues: {
      account_ids: props.accountID ? [props.accountID] : [],
      unified_model: "",
      timeout_seconds: 120,
    },
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const policy = useQuery({ queryKey: ["policy"], queryFn: api.policy });
  const manualPriorityTask = useQuery({
    queryKey: ["model-animation", "manual-priority-task", manualPriorityTaskID],
    queryFn: () => api.task(manualPriorityTaskID!),
    enabled: Boolean(manualPriorityTaskID),
    refetchInterval: taskPollInterval,
  });
  const manualPriorityMutation = useMutation({
    mutationFn: (input: ManualPriorityValues | null) => {
      if (input === null) return api.clearAccountManualPriority(manualPriorityAccount!.id);
      return api.setAccountManualPriority(
        manualPriorityAccount!.id,
        input.priority,
        input.loadFactor,
        input.concurrency,
        input.schedulable,
        input.syncBalanceMultiplier,
      );
    },
    onSuccess: (task) => setManualPriorityTaskID(task.id),
    onError: (error) => notifyOperationError(error, "手动控制操作失败"),
  });
  useEffect(() => {
    const task = manualPriorityTask.data;
    if (!task || ["queued", "running", "waiting_input"].includes(task.status)) return;
    setManualPriorityTaskID(null);
    setManualPriorityAccount(null);
    void client.invalidateQueries({ queryKey: ["accounts"] });
    if (task.status === "succeeded") toast.success("手动控制已更新");
    else toast.error(task.message || "手动控制操作失败");
  }, [client, manualPriorityTask.data]);
  const manualPrioritySection = policy.data?.advanced_policy?.manual_priority;
  const configuredReservedMax =
    manualPrioritySection !== null &&
    typeof manualPrioritySection === "object" &&
    !Array.isArray(manualPrioritySection)
      ? (manualPrioritySection as Record<string, unknown>).reserved_max
      : null;
  const reservedMax =
    typeof configuredReservedMax === "number" &&
    Number.isInteger(configuredReservedMax) &&
    configuredReservedMax >= 1
      ? configuredReservedMax
      : 10;
  const schedules = useQuery({
    queryKey: ["model-animation", "schedules"],
    queryFn: api.animationSchedules,
    refetchInterval: props.active === false ? false : 15_000,
  });
  const run = useMutation({
    mutationFn: tasks.start,
  });
  const submit = (value: AnimationForm, mode?: "precheck"): void => {
    run.mutate({
      ...(mode ? { mode, precheck_questions: precheckQuestions } : {}),
      targets: value.account_ids
        .filter((id) => !tasks.busyIDs.has(id))
        .map((id) => ({
          account_id: id,
          model: value.unified_model.trim(),
        })),
      timeout_seconds: value.timeout_seconds,
    });
  };
  const editSchedules = (selected: AccountStatus[], batch: boolean): void => {
    const mode = tab === "precheck" ? "precheck" : "animation";
    setScheduleEdit({
      mode,
      batch,
      targets: selected.map((account) => ({
        accountID: account.id,
        accountName: account.name,
        schedule: schedules.data?.find(
          (item) => item.account_id === account.id && (item.mode ?? "animation") === mode,
        ),
      })),
    });
  };
  const start = tasks.start;
  const retry = useCallback(
    (target: AnimationTarget): void => {
      const timeout = form.getValues("timeout_seconds");
      void start({
        targets: [target],
        timeout_seconds: Number.isFinite(timeout) && timeout >= 5 && timeout <= 120 ? timeout : 120,
      });
    },
    [form, start],
  );
  return (
    <>
      <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
        <Tabs.Root
          value={tab}
          onValueChange={(value) => setTab(String(value))}
          className="flex min-h-0 flex-1 flex-col"
        >
          <Tabs.List
            aria-label="动画检测来源"
            className="flex shrink-0 flex-wrap gap-1 border-b px-3"
          >
            <Tabs.Tab
              value="accounts"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              账号检测
            </Tabs.Tab>
            <Tabs.Tab
              value="precheck"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              前置检测
            </Tabs.Tab>
            <Tabs.Tab
              value="custom"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              自定义接口
            </Tabs.Tab>
            <Tabs.Tab
              value="terminal-continuity"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              终端续接检测
            </Tabs.Tab>
            <Tabs.Tab
              value="task-management"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              任务管理
            </Tabs.Tab>
          </Tabs.List>
          <Tabs.Panel
            value={tab === "precheck" ? "precheck" : "accounts"}
            keepMounted
            className="min-h-0 flex-1 data-[hidden]:hidden"
          >
            <AnimationSelection
              filters={filters}
              onFiltersChange={setFilters}
              mode={tab === "precheck" ? "precheck" : "animation"}
              accountID={props.showAllAccounts ? undefined : props.accountID}
              precheckQuestions={precheckQuestions}
              onPrecheckQuestionsChange={setPrecheckQuestions}
              precheckResults={tasks.precheckResults}
              precheckStatuses={tasks.precheckStatuses}
              onPrecheck={(value) => submit(value, "precheck")}
              results={tasks.results}
              statuses={tasks.statuses}
              activities={tasks.activities}
              busyIDs={tasks.busyIDs}
              onRetry={retry}
              taskRetry={
                <>
                  {tasks.historyError ? (
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            aria-label="重新读取检测记录"
                            disabled={tasks.retryingHistory}
                            onClick={tasks.retryHistory}
                          />
                        }
                      >
                        <RefreshCw aria-hidden="true" />
                      </TooltipTrigger>
                      <TooltipContent>重新读取检测记录</TooltipContent>
                    </Tooltip>
                  ) : null}
                  {tasks.activeTaskIDs.size > 0 ? (
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            aria-label="取消任务"
                            disabled={tasks.cancelling}
                            onClick={() => void tasks.cancelActive()}
                          />
                        }
                      >
                        <Square aria-hidden="true" />
                      </TooltipTrigger>
                      <TooltipContent>取消任务</TooltipContent>
                    </Tooltip>
                  ) : null}
                </>
              }
              form={form}
              accounts={accounts}
              schedules={schedules}
              pending={run.isPending}
              onSubmit={(value) => submit(value)}
              onSchedule={(account) => editSchedules([account], false)}
              onBatchSchedule={(accounts) => editSchedules(accounts, true)}
              onManualPriority={setManualPriorityAccount}
            />
          </Tabs.Panel>
          <Tabs.Panel value="custom" className="min-h-0 flex-1">
            <CustomAnimationPanel tasks={tasks} active={props.active} />
          </Tabs.Panel>
          <Tabs.Panel value="task-management" className="min-h-0 flex-1">
            <Suspense fallback={<ContentLoading label="正在读取任务管理" />}>
              <DetectionTaskPanel />
            </Suspense>
          </Tabs.Panel>
          <Tabs.Panel value="terminal-continuity" className="min-h-0 flex-1">
            <Suspense
              fallback={
                <ContentLoading label="正在读取终端续接检测" className="h-full justify-center" />
              }
            >
              <TerminalContinuityPanel
                accountID={props.showAllAccounts ? undefined : props.accountID}
                filters={filters}
                onFiltersChange={setFilters}
              />
            </Suspense>
          </Tabs.Panel>
        </Tabs.Root>
      </div>
      {scheduleEdit ? (
        <AnimationScheduleDialog
          accountID={scheduleEdit.targets[0].accountID}
          accountName={
            scheduleEdit.batch
              ? `${scheduleEdit.targets.length} 个账号`
              : scheduleEdit.targets[0].accountName
          }
          mode={scheduleEdit.mode}
          targets={scheduleEdit.batch ? scheduleEdit.targets : undefined}
          model={form.getValues("unified_model").trim()}
          schedule={scheduleEdit.batch ? undefined : scheduleEdit.targets[0].schedule}
          onClose={() => setScheduleEdit(null)}
        />
      ) : null}
      {manualPriorityAccount ? (
        <AccountManualPriorityDialog
          open
          account={manualPriorityAccount}
          reservedMax={reservedMax}
          pending={manualPriorityMutation.isPending || Boolean(manualPriorityTaskID)}
          onOpenChange={(open) => {
            if (!open && !manualPriorityMutation.isPending && !manualPriorityTaskID)
              setManualPriorityAccount(null);
          }}
          onAssign={(values) => manualPriorityMutation.mutate(values)}
          onClear={() => manualPriorityMutation.mutate(null)}
        />
      ) : null}
    </>
  );
}
