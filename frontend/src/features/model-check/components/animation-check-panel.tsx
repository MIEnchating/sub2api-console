import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { api, type AccountStatus, type AnimationRequest, type AnimationTarget } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { useAnimationTasks } from "../hooks/use-animation-tasks";
import { animationSchema, type AnimationForm } from "../lib/animation-schema";
import { AnimationSelection } from "./animation-selection";
import { RefreshCw, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AnimationScheduleDialog } from "./animation-schedule-dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function AnimationCheckPanel(props: { active?: boolean }): ReactElement {
  const tasks = useAnimationTasks(props.active);
  const [scheduleAccount, setScheduleAccount] = useState<AccountStatus | null>(null);
  const [confirmation, setConfirmation] = useState<AnimationRequest | null>(null);
  const form = useForm<AnimationForm>({
    resolver: zodResolver(animationSchema),
    defaultValues: {
      account_ids: [],
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
  const submit = (value: AnimationForm): void => {
    setConfirmation({
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
      <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border bg-card">
        <AnimationSelection
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
                <Button
                  type="button"
                  variant="outline"
                  aria-label="取消任务"
                  disabled={tasks.cancelling}
                  onClick={() => void tasks.cancelActive()}
                >
                  <X aria-hidden="true" />
                  取消任务
                </Button>
              ) : null}
            </>
          }
          form={form}
          accounts={accounts}
          schedules={schedules}
          pending={run.isPending}
          onSubmit={submit}
          onSchedule={setScheduleAccount}
        />
      </div>
      <ConfirmActionDialog
        open={confirmation !== null}
        title="确认动画检测范围"
        description={`将检测 ${confirmation?.targets.length ?? 0} 个账号并产生 API 用量：${confirmation?.targets.map((target) => `${accounts.data?.find((account) => account.id === target.account_id)?.name ?? target.account_id}（ID ${target.account_id}）→ ${target.model}`).join("；") ?? ""}。`}
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
          key={`${scheduleAccount.id}-${schedules.data?.find((item) => item.account_id === scheduleAccount.id)?.version ?? 0}`}
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
