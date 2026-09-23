import { memo, type ReactElement } from "react";
import type {
  AccountStatus,
  AnimationResult,
  AnimationSchedule,
  AnimationTarget,
  Task,
} from "@/api";
import { Pin, ScanLine, Settings2 } from "lucide-react";
import { ContentLoading } from "@/components/content-loading";
import { AnimationAccountResult } from "./animation-account-result";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { AnimationActivity } from "../lib/animation-task-results";
import { cn } from "@/lib/utils";
import { PrecheckAccountResult } from "./precheck-account-result";
import { DetectionAccountControls } from "./detection-account-controls";

export const AnimationAccountCard = memo(function AnimationAccountCard(props: {
  mode?: "animation" | "precheck";
  account: AccountStatus;
  result?: AnimationResult;
  precheckResult?: AnimationResult;
  precheckStatus?: Task["status"];
  taskStatus?: Task["status"];
  activity?: AnimationActivity;
  retryDisabled: boolean;
  onRetry: (target: AnimationTarget) => void;
  checked: boolean;
  disabled: boolean;
  schedule?: AnimationSchedule;
  schedulesReady: boolean;
  onToggle: (id: string, checked: boolean) => void;
  onSchedule: (account: AccountStatus) => void;
  onManualPriority?: (account: AccountStatus) => void;
}): ReactElement {
  const precheckMode = props.mode === "precheck";
  const unavailable =
    props.account.platform != null && !["openai", "anthropic"].includes(props.account.platform);
  const currentStatus = precheckMode ? props.precheckStatus : props.taskStatus;
  const activityMatchesMode = (props.activity?.mode === "precheck") === precheckMode;
  const currentActivity = activityMatchesMode ? props.activity : undefined;
  const running =
    Boolean(currentActivity) ||
    currentStatus === "queued" ||
    currentStatus === "running" ||
    currentStatus === "waiting_input";
  const groups = props.account.groups.join("、");
  let scheduleLabel = props.schedule?.enabled
    ? `每 ${props.schedule.interval_minutes} 分钟自动检测`
    : "自动检测关闭";
  if (running && (precheckMode ? props.precheckResult : props.result))
    scheduleLabel = "正在重新检测";
  if (props.schedule?.enabled && props.schedule.mode === "precheck")
    scheduleLabel = `每 ${props.schedule.interval_minutes} 分钟前置检测`;
  if (props.schedule?.enabled && props.schedule.mode === "both")
    scheduleLabel = `每 ${props.schedule.interval_minutes} 分钟前置与动画检测`;
  if (props.schedule?.enabled && props.schedule.schedule_type === "daily")
    scheduleLabel = `每天 ${(props.schedule.daily_times ?? [props.schedule.daily_time]).join("、")}（北京时间）自动检测`;
  if (props.schedule?.last_error)
    scheduleLabel = `最近自动检测未启动：${props.schedule.last_error}`;
  if (
    !running &&
    props.retryDisabled &&
    (currentStatus === "succeeded" || currentStatus === "failed")
  )
    scheduleLabel = "本项检测已结束，等待任务结束";
  return (
    <article
      aria-label={`账号 ${props.account.name}`}
      className={cn(
        "flex h-auto min-w-0 flex-col overflow-hidden rounded-lg border border-border/70 bg-card transition-colors hover:border-border",
        props.checked && "border-primary/60 bg-primary/[0.02] ring-1 ring-primary/10",
      )}
    >
      <header className="flex h-24 shrink-0 flex-col justify-center gap-1.5 px-3">
        <label className="flex h-5 min-w-0 items-center gap-2">
          <Checkbox
            checked={props.checked}
            disabled={props.disabled || unavailable}
            onCheckedChange={(checked) => props.onToggle(props.account.id, checked)}
            aria-label={`检测 ${props.account.name}`}
          />
          <Tooltip>
            <TooltipTrigger
              render={<span className="min-w-0 flex-1 truncate text-sm font-medium" />}
            >
              {props.account.name}
            </TooltipTrigger>
            <TooltipContent>{props.account.name}</TooltipContent>
          </Tooltip>
          <span className="text-muted-foreground shrink-0 text-xs">ID {props.account.id}</span>
        </label>
        <div className="flex h-5 min-w-0 items-center gap-1.5 overflow-hidden">
          <Badge variant="outline">{props.account.platform ?? "未标注平台"}</Badge>
          {props.account.manual_priority != null ? (
            <Badge variant="secondary">手动控制 #{props.account.manual_priority}</Badge>
          ) : null}
        </div>
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <Tooltip>
            <TooltipTrigger render={<span className="min-w-0 flex-1 truncate" />}>
              {groups || "未加入分组"}
            </TooltipTrigger>
            <TooltipContent>{groups || "未加入分组"}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger render={<span className="min-w-0 flex-1 truncate text-right" />}>
              {props.account.upstream_host || "未配置 Host"}
            </TooltipTrigger>
            <TooltipContent>{props.account.upstream_host || "未配置 Host"}</TooltipContent>
          </Tooltip>
        </div>
      </header>
      {precheckMode ? (
        <div className="flex min-h-14 min-w-0 shrink-0 items-center border-y border-border/40 bg-muted/10 px-3">
          <PrecheckAccountResult
            result={props.precheckResult}
            status={props.precheckStatus}
            activity={currentActivity}
          />
        </div>
      ) : (
        <div className="min-w-0 shrink-0">
          {props.result ? (
            <AnimationAccountResult
              layout="card"
              result={props.result}
              activity={currentActivity}
              retryDisabled={props.retryDisabled || running || unavailable}
              onRetry={props.onRetry}
            />
          ) : (
            <>
              <div
                role="group"
                aria-label="动画预览区域"
                className="h-[180px] w-full shrink-0 overflow-hidden border-y border-border/40 bg-muted/20"
              >
                <AnimationCardState
                  status={props.taskStatus}
                  activity={currentActivity}
                  unavailable={unavailable}
                />
              </div>
              <div className="flex h-14 items-center px-3 text-xs text-muted-foreground">
                动画检测：暂无结果
              </div>
            </>
          )}
        </div>
      )}
      <footer className="flex h-10 shrink-0 items-center justify-between gap-2 border-t border-border/60 bg-muted/20 px-3">
        <Tooltip>
          <TooltipTrigger
            render={
              <p
                role={running && props.result ? "status" : undefined}
                className={cn(
                  "min-w-0 truncate text-xs text-muted-foreground",
                  props.schedule?.last_error && "text-destructive",
                )}
              />
            }
          >
            {scheduleLabel}
          </TooltipTrigger>
          <TooltipContent>{scheduleLabel}</TooltipContent>
        </Tooltip>
        <div className="flex shrink-0 items-center gap-1">
          <DetectionAccountControls account={props.account} />
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={
                    props.account.manual_priority == null ? "设置手动控制" : "调整手动控制"
                  }
                  disabled={unavailable || !props.onManualPriority}
                  onClick={() => props.onManualPriority?.(props.account)}
                />
              }
            >
              <Pin aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>
              {props.account.manual_priority == null ? "设置手动控制" : "调整手动控制"}
            </TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="自动检测设置"
                  disabled={!props.schedulesReady || unavailable}
                  onClick={() => props.onSchedule(props.account)}
                />
              }
            >
              <Settings2 aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>自动检测设置</TooltipContent>
          </Tooltip>
        </div>
      </footer>
    </article>
  );
});

function AnimationCardState(props: {
  status?: Task["status"];
  activity?: AnimationActivity;
  unavailable: boolean;
}): ReactElement {
  if (props.activity) {
    let label = "生成中，等待动画结果";
    if (props.activity.status === "starting") label = "正在启动检测";
    return (
      <ContentLoading compact label={label} ariaLabel={label} className="h-full justify-center" />
    );
  }
  if (props.status === "queued" || props.status === "running" || props.status === "waiting_input")
    return (
      <ContentLoading compact label="正在检测，等待动画结果" className="h-full justify-center" />
    );
  let label = "";
  if (props.unavailable) label = "当前接口类型不支持动画检测";
  else if (props.status === "cancelled") label = "检测已取消，未返回动画";
  else if (props.status) label = "本次检测未返回动画";
  if (!label)
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 px-3 text-center text-muted-foreground">
        <ScanLine className="size-6" aria-hidden="true" />
        <p className="text-xs">尚未检测</p>
      </div>
    );
  return (
    <p className="flex h-full items-center justify-center px-3 text-center text-xs text-muted-foreground">
      {label}
    </p>
  );
}
