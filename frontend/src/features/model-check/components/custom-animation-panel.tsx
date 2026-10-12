import { animationTestModel } from "../constants";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  ChevronLeft,
  ChevronRight,
  Clapperboard,
  Play,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { useEffect, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { AnimationHistorySkeleton } from "./animation-accounts-skeleton";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { useIsMobile } from "@/hooks/use-mobile";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { customAnimationSchema, type CustomAnimationForm } from "../lib/animation-schema";
import { CustomAnimationRecord } from "./custom-animation-record";
import { AnimationPromptDialog } from "./animation-prompt-dialog";
import { CustomAnimationParameters } from "./custom-animation-parameters";
import { CustomAnimationFields } from "./custom-animation-fields";
import type { PrecheckQuestionID } from "@/api";
import { allPrecheckQuestions, precheckQuestionSummary } from "../constants";
import { PrecheckQuestionSelector } from "./precheck-question-selector";

export function CustomAnimationPanel(props: {
  tasks: ReturnType<typeof useAnimationTasks>;
  active?: boolean;
}): ReactElement {
  const [confirmation, setConfirmation] = useState(false);
  const [mode, setMode] = useState<"animation" | "precheck">("animation");
  const [questions, setQuestions] = useState<PrecheckQuestionID[]>(allPrecheckQuestions);
  const [pending, setPending] = useState(false);
  const form = useForm<CustomAnimationForm>({
    resolver: zodResolver(customAnimationSchema),
    defaultValues: {
      base_url: "",
      api_key: "",
      platform: "openai",
      model: animationTestModel,
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
        ...(mode === "precheck" ? { mode, precheck_questions: questions } : {}),
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
  const ids = [
    ...new Set([
      ...props.tasks.results.keys(),
      ...props.tasks.statuses.keys(),
      ...props.tasks.precheckResults.keys(),
      ...props.tasks.precheckStatuses.keys(),
    ]),
  ]
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
        className="shrink-0 border-b"
        noValidate
        autoComplete="off"
        onSubmit={form.handleSubmit(() => {
          setMode("animation");
          setConfirmation(true);
        })}
      >
        <div className="px-3 py-3 sm:px-4">
          <CustomAnimationFields form={form} disabled={pending || confirmation} />
        </div>
        <div
          role="group"
          aria-label="自定义接口检测操作"
          className="flex min-w-0 flex-wrap items-center gap-2 border-t bg-muted/20 px-3 py-2 sm:px-4"
        >
          <CustomAnimationParameters form={form} disabled={pending || confirmation} />
          {mode === "animation" ? (
            <AnimationPromptDialog disabled={pending || confirmation} />
          ) : null}
          <PrecheckQuestionSelector
            value={questions}
            onChange={setQuestions}
            disabled={pending || confirmation}
          />
          <Button type="button" variant="outline" disabled={pending} onClick={() => form.reset()}>
            <RefreshCw aria-hidden="true" />
            清空配置
          </Button>
          <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={pending || questions.length === 0}
              onClick={() =>
                void form.handleSubmit(() => {
                  setMode("precheck");
                  setConfirmation(true);
                })()
              }
            >
              <ShieldCheck aria-hidden="true" />
              前置检测
            </Button>
            <Button type="submit" disabled={pending} aria-busy={pending}>
              <Play aria-hidden="true" />
              开始检测
            </Button>
          </div>
        </div>
      </form>
      <section
        aria-label="自定义接口检测记录"
        className="flex min-h-48 flex-1 flex-col gap-3 p-3 sm:p-4"
      >
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h3 className="text-sm font-medium">检测记录</h3>
          <span className="text-xs tabular-nums text-muted-foreground">共 {ids.length} 条</span>
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
        {props.tasks.historyError ? (
          <ContentRetry onRetry={props.tasks.retryHistory} pending={props.tasks.retryingHistory} />
        ) : null}
        {pending ? <TaskStartupState message="正在启动自定义接口检测" /> : null}
        {!pending &&
        !props.tasks.historyLoading &&
        !props.tasks.historyError &&
        ids.length === 0 ? (
          <div
            role="group"
            aria-label="暂无检测记录"
            className="flex min-h-40 shrink-0 flex-col items-center justify-center gap-2 rounded-lg bg-muted/10 px-4 py-6 text-center"
          >
            <Clapperboard aria-hidden="true" className="mb-1 size-6 text-muted-foreground" />
            <p className="text-sm font-medium">暂无自定义接口检测记录</p>
            <p className="text-xs text-muted-foreground">
              填写接口信息后开始检测，结果将在这里展示。
            </p>
          </div>
        ) : null}
        {ids.length > 0 || props.tasks.historyLoading ? (
          <div className="grid shrink-0 grid-cols-[repeat(auto-fit,minmax(min(100%,20rem),1fr))] items-start gap-3">
            {props.tasks.historyLoading && ids.length === 0 ? <AnimationHistorySkeleton /> : null}
            {pagination.visibleItems.map((id) => (
              <CustomAnimationRecord
                key={id}
                id={id}
                tasks={props.tasks}
                retryDisabled={pending}
                onRetry={(model) => {
                  form.setValue("model", model);
                  setMode("animation");
                  form.setFocus("api_key");
                  void form.handleSubmit(() => setConfirmation(true))();
                }}
              />
            ))}
          </div>
        ) : null}
      </section>
      <ConfirmActionDialog
        open={confirmation}
        title="确认自定义接口检测"
        description={`将向 ${form.getValues("base_url")} 发送 ${form.getValues("platform")} ${mode === "precheck" ? `前置检测请求（${precheckQuestionSummary(questions)}）` : "动画生成请求"}，模型为 ${form.getValues("model")}，并产生 API 用量。`}
        confirmLabel="确认并开始检测"
        pending={pending}
        onOpenChange={setConfirmation}
        onConfirm={() => void submit()}
      />
    </div>
  );
}
