import { zodResolver } from "@hookform/resolvers/zod";
import { Tabs } from "@base-ui/react/tabs";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import {
  api,
  type AccountStatus,
  type AnimationRequest,
  type AnimationTarget,
  type PrecheckQuestionID,
} from "@/api";
import { allPrecheckQuestions, precheckQuestionSummary } from "../constants";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { useAnimationTasks } from "../hooks/use-animation-tasks";
import { animationSchema, type AnimationForm } from "../lib/animation-schema";
import { AnimationSelection } from "./animation-selection";
import { RefreshCw, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AnimationScheduleDialog } from "./animation-schedule-dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { CustomAnimationPanel } from "./custom-animation-panel";

export function AnimationCheckPanel(props: {
  active?: boolean;
  accountID?: string;
  showAllAccounts?: boolean;
}): ReactElement {
  const tasks = useAnimationTasks(props.active);
  const [scheduleAccount, setScheduleAccount] = useState<AccountStatus | null>(null);
  const [confirmation, setConfirmation] = useState<AnimationRequest | null>(null);
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
  const schedules = useQuery({
    queryKey: ["model-animation", "schedules"],
    queryFn: api.animationSchedules,
    refetchInterval: props.active === false ? false : 15_000,
  });
  const run = useMutation({
    mutationFn: tasks.start,
    onSuccess: () => setConfirmation(null),
  });
  const submit = (value: AnimationForm, mode?: "precheck"): void => {
    setConfirmation({
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
        <Tabs.Root defaultValue="accounts" className="flex min-h-0 flex-1 flex-col">
          <Tabs.List aria-label="动画检测来源" className="flex shrink-0 gap-1 border-b px-3">
            <Tabs.Tab
              value="accounts"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              账号检测
            </Tabs.Tab>
            <Tabs.Tab
              value="custom"
              className="border-b-2 border-transparent px-3 py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              自定义接口
            </Tabs.Tab>
          </Tabs.List>
          <Tabs.Panel value="accounts" keepMounted className="min-h-0 flex-1 data-[hidden]:hidden">
            <AnimationSelection
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
              onSchedule={setScheduleAccount}
            />
          </Tabs.Panel>
          <Tabs.Panel value="custom" className="min-h-0 flex-1">
            <CustomAnimationPanel tasks={tasks} active={props.active} />
          </Tabs.Panel>
        </Tabs.Root>
      </div>
      <ConfirmActionDialog
        open={confirmation !== null}
        title={confirmation?.mode === "precheck" ? "确认前置检测范围" : "确认动画检测范围"}
        description={`将${confirmation?.mode === "precheck" ? `对每个账号执行${precheckQuestionSummary(confirmation.precheck_questions)}，` : ""}检测 ${confirmation?.targets.length ?? 0} 个账号并产生 API 用量：${confirmation?.targets.map((target) => `${accounts.data?.find((account) => account.id === target.account_id)?.name ?? target.account_id}（ID ${target.account_id}）→ ${target.model}`).join("；") ?? ""}。`}
        confirmLabel="确认并开始检测"
        pending={run.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation) run.mutate(confirmation);
        }}
      />
      {scheduleAccount ? (
        <AnimationScheduleDialog
          key={scheduleAccount.id}
          accountID={scheduleAccount.id}
          accountName={scheduleAccount.name}
          model={form.getValues("unified_model").trim()}
          schedule={schedules.data?.find((item) => item.account_id === scheduleAccount.id)}
          onClose={() => setScheduleAccount(null)}
        />
      ) : null}
    </>
  );
}
