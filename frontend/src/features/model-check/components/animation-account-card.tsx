import { memo, type ReactElement } from "react";
import type {
  AccountStatus,
  AnimationResult,
  AnimationSchedule,
  AnimationTarget,
  Task,
} from "@/api";
import { ScanLine, Settings2 } from "lucide-react";
import { ContentLoading } from "@/components/content-loading";
import { AnimationAccountResult } from "./animation-account-result";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { AnimationActivity } from "../lib/animation-task-results";
import { cn } from "@/lib/utils";
import { PrecheckAccountResult } from "./precheck-account-result";

export const AnimationAccountCard = memo(function AnimationAccountCard(props: {
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
}): ReactElement {
  const unavailable =
    props.account.platform != null && !["openai", "anthropic"].includes(props.account.platform);
  const running =
    Boolean(props.activity) ||
    props.taskStatus === "queued" ||
    props.taskStatus === "running" ||
    props.taskStatus === "waiting_input";
  const groups = props.account.groups.join("、");
  const animationActivity = props.activity?.mode === "precheck" ? undefined : props.activity;
  let scheduleLabel = props.schedule?.enabled
    ? `每 ${props.schedule.interval_minutes} 分钟自动检测`
    : "自动检测关闭";
  if (running && props.result) scheduleLabel = "正在重新检测";
  if (props.schedule?.enabled && props.schedule.mode === "precheck")
    scheduleLabel = `每 ${props.schedule.interval_minutes} 分钟前置检测`;
  if (props.schedule?.enabled && props.schedule.mode === "both")
    scheduleLabel = `每 ${props.schedule.interval_minutes} 分钟前置与动画检测`;
  if (props.schedule?.last_error)
    scheduleLabel = `最近自动检测未启动：${props.schedule.last_error}`;
  return (
    <article
      aria-label={`账号 ${props.account.name}`}
      className={cn(
        "flex h-auto min-w-0 flex-col overflow-hidden rounded-lg border border-border/70 bg-card transition-colors hover:border-border",
        props.checked && "border-primary/60 bg-primary/[0.02] ring-1 ring-primary/10",
      )}
    >
      <header className="flex h-[72px] shrink-0 flex-col justify-center gap-0.5 px-3">
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
        <div className="flex h-4 min-w-0 items-center gap-1.5">
          <Badge variant="outline">{props.account.platform ?? "未标注平台"}</Badge>
          {props.account.manual_priority != null ? (
            <Badge variant="secondary">人工优先</Badge>
          ) : null}
          <Tooltip>
            <TooltipTrigger
              render={<span className="min-w-0 truncate text-xs text-muted-foreground" />}
            >
              {groups}
            </TooltipTrigger>
            <TooltipContent>{groups || "未加入分组"}</TooltipContent>
          </Tooltip>
        </div>
        <Tooltip>
          <TooltipTrigger render={<p className="truncate text-xs text-muted-foreground" />}>
            {props.account.upstream_host || "未配置 Host"}
          </TooltipTrigger>
          <TooltipContent>{props.account.upstream_host || "未配置 Host"}</TooltipContent>
        </Tooltip>
      </header>
      <div className="h-56 min-w-0 shrink-0">
        {props.result ? (
          <AnimationAccountResult
            layout="card"
            result={props.result}
            activity={animationActivity}
            retryDisabled={props.retryDisabled}
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
                activity={animationActivity}
                unavailable={unavailable}
              />
            </div>
            <div className="flex h-11 items-center px-3 text-xs text-muted-foreground">
              动画检测：暂无结果
            </div>
          </>
        )}
      </div>
      <div className="h-auto shrink-0 border-t border-border/40 px-3">
        <PrecheckAccountResult
          result={props.precheckResult}
          status={props.precheckStatus}
          activity={props.activity}
        />
      </div>
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
