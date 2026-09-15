import { zodResolver } from "@hookform/resolvers/zod";
import { ChevronLeft, ChevronRight, Play, RefreshCw } from "lucide-react";
import { useEffect, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { AnimationHistorySkeleton } from "./animation-accounts-skeleton";
import { TaskCancelButton, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { useIsMobile } from "@/hooks/use-mobile";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { customAnimationSchema, type CustomAnimationForm } from "../lib/animation-schema";
import { AnimationAccountResult } from "./animation-account-result";
import { CustomAnimationFields } from "./custom-animation-fields";

export function CustomAnimationPanel(props: {
  tasks: ReturnType<typeof useAnimationTasks>;
  active?: boolean;
}): ReactElement {
  const [confirmation, setConfirmation] = useState(false);
  const [pending, setPending] = useState(false);
  const form = useForm<CustomAnimationForm>({
    resolver: zodResolver(customAnimationSchema),
    defaultValues: {
      base_url: "",
      api_key: "",
      platform: "openai",
      model: "",
      timeout_seconds: 120,
    },
  });
  useEffect(() => {
    if (props.active === false) {
      form.setValue("api_key", "");
      setConfirmation(false);
    }
  }, [form, props.active]);
  const submit = form.handleSubmit(async (value) => {
    if (pending) return;
    setPending(true);
    try {
      const started = await props.tasks.start({
        targets: [],
        timeout_seconds: value.timeout_seconds,
        custom: {
          base_url: value.base_url,
          api_key: value.api_key,
          platform: value.platform,
          model: value.model,
        },
      });
      if (started) form.setValue("api_key", "");
    } finally {
      setPending(false);
      setConfirmation(false);
    }
  });
  const ids = [...new Set([...props.tasks.results.keys(), ...props.tasks.statuses.keys()])]
    .filter((id) => id.startsWith("custom-"))
    .reverse();
  const mobile = useIsMobile();
  const pagination = useClientPagination(ids, 3);
  const setPageSize = pagination.setPageSize;
  const setCurrentPage = pagination.setCurrentPage;
  const newestID = ids[0];
  useEffect(() => setPageSize(mobile ? 1 : 3), [mobile, setPageSize]);
  useEffect(() => setCurrentPage(1), [newestID, setCurrentPage]);
  return (
    <div
      role="region"
      aria-label="自定义动画检测内容"
      className="flex h-full min-h-0 flex-col overflow-y-auto overscroll-contain"
    >
      <form
        className="shrink-0 space-y-2 border-b p-3"
        noValidate
        autoComplete="off"
        onSubmit={form.handleSubmit(() => setConfirmation(true))}
      >
        <CustomAnimationFields form={form} disabled={pending || confirmation} />
        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" disabled={pending} aria-busy={pending}>
            <Play aria-hidden="true" />
            开始检测
          </Button>
          <Button type="button" variant="outline" disabled={pending} onClick={() => form.reset()}>
            <RefreshCw aria-hidden="true" />
            清空配置
          </Button>
          {pagination.totalPages > 1 ? (
            <nav aria-label="自定义检测记录分页" className="ml-auto flex items-center gap-1">
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      aria-label="上一页记录"
                      disabled={pagination.currentPage === 1}
                      onClick={() => setCurrentPage(pagination.currentPage - 1)}
                    />
                  }
                >
                  <ChevronLeft aria-hidden="true" />
                </TooltipTrigger>
                <TooltipContent>上一页记录</TooltipContent>
              </Tooltip>
              <span className="text-xs tabular-nums" aria-live="polite">
                {pagination.currentPage}/{pagination.totalPages}
              </span>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      aria-label="下一页记录"
                      disabled={pagination.currentPage === pagination.totalPages}
                      onClick={() => setCurrentPage(pagination.currentPage + 1)}
                    />
                  }
                >
                  <ChevronRight aria-hidden="true" />
                </TooltipTrigger>
                <TooltipContent>下一页记录</TooltipContent>
              </Tooltip>
            </nav>
          ) : null}
        </div>
      </form>
      <section aria-label="自定义接口检测记录" className="flex min-h-48 flex-1 flex-col gap-2 p-3">
        {props.tasks.historyError ? (
          <ContentRetry onRetry={props.tasks.retryHistory} pending={props.tasks.retryingHistory} />
        ) : null}
        {pending ? <TaskStartupState message="正在启动自定义接口检测" /> : null}
        {!pending &&
        !props.tasks.historyLoading &&
        !props.tasks.historyError &&
        ids.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无自定义接口检测记录</p>
        ) : null}
        <div className="grid min-h-0 flex-1 grid-rows-1 gap-3 md:grid-cols-3">
          {props.tasks.historyLoading && ids.length === 0 ? <AnimationHistorySkeleton /> : null}
          {pagination.visibleItems.map((id) => {
            const activity = props.tasks.activities.get(id);
            const result = props.tasks.results.get(id);
            return (
              <article
                key={id}
                aria-label={`自定义检测 ${result?.model ?? id}`}
                className="flex min-h-0 min-w-0 flex-col gap-2 rounded-lg border p-2"
              >
                {activity ? (
                  <>
                    <TaskStartupState message="生成中，等待动画结果" />
                    {activity.taskID ? <TaskCancelButton taskId={activity.taskID} /> : null}
                  </>
                ) : null}
                {!activity && result ? (
                  <AnimationAccountResult
                    result={result}
                    retryDisabled={pending}
                    onRetry={(target) => {
                      form.setValue("model", target.model);
                      form.setFocus("api_key");
                      void form.handleSubmit(() => setConfirmation(true))();
                    }}
                  />
                ) : null}
                {!activity && !result ? (
                  <p className="text-sm text-muted-foreground">检测已结束，未返回动画</p>
                ) : null}
              </article>
            );
          })}
        </div>
      </section>
      <ConfirmActionDialog
        open={confirmation}
        title="确认自定义接口检测"
        description={`将向 ${form.getValues("base_url")} 发送 ${form.getValues("platform")} 动画生成请求，模型为 ${form.getValues("model")}，并产生 API 用量。`}
        confirmLabel="确认并开始检测"
        pending={pending}
        onOpenChange={setConfirmation}
        onConfirm={() => void submit()}
      />
    </div>
  );
}
