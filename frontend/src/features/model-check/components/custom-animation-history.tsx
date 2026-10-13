import { Tabs } from "@base-ui/react/tabs";
import { Clapperboard } from "lucide-react";
import type { ReactElement } from "react";
import { ContentRetry } from "@/components/content-retry";
import { TaskStartupState } from "@/components/task-startup-state";
import { detectionStageOptions } from "../constants";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { AnimationHistorySkeleton } from "./animation-accounts-skeleton";
import { CustomAnimationRecords } from "./custom-animation-records";

export function CustomAnimationHistory(props: {
  tasks: ReturnType<typeof useAnimationTasks>;
  pending: boolean;
  mode: "animation" | "precheck";
  onModeChange: (mode: "animation" | "precheck") => void;
  onRetry: (model: string) => void;
}): ReactElement {
  const hasRecords = [...props.tasks.statuses.keys(), ...props.tasks.precheckStatuses.keys()].some(
    (id) => id.startsWith("custom-"),
  );
  return (
    <section aria-label="自定义接口检测记录" className="min-h-48 flex-1 p-3 sm:p-4">
      <Tabs.Root
        value={props.mode}
        onValueChange={(value) => {
          if (value === "animation" || value === "precheck") props.onModeChange(value);
        }}
        className="flex min-w-0 flex-col gap-3"
      >
        <Tabs.List
          aria-label="自定义检测记录类型"
          className="flex w-fit gap-1 rounded-lg bg-muted p-1"
        >
          {detectionStageOptions.map((stage) => (
            <Tabs.Tab
              key={stage.value}
              value={stage.value}
              className="h-8 rounded-md px-3 text-sm outline-none data-active:bg-background data-active:shadow-sm focus-visible:ring-2 focus-visible:ring-ring"
            >
              {stage.label}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        {props.tasks.historyError ? (
          <ContentRetry onRetry={props.tasks.retryHistory} pending={props.tasks.retryingHistory} />
        ) : null}
        {props.pending ? <TaskStartupState message="正在启动自定义接口检测" /> : null}
        {!props.pending &&
        !props.tasks.historyLoading &&
        !props.tasks.historyError &&
        !hasRecords ? (
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
        {props.tasks.historyLoading && !hasRecords ? (
          <div className="grid shrink-0 grid-cols-1 items-start gap-3 md:grid-cols-3">
            <AnimationHistorySkeleton />
          </div>
        ) : null}
        {detectionStageOptions.map((stage) => (
          <Tabs.Panel
            key={stage.value}
            value={stage.value}
            keepMounted
            className="min-w-0 data-[hidden]:hidden"
          >
            <CustomAnimationRecords
              mode={stage.value}
              tasks={props.tasks}
              retryDisabled={props.pending}
              onRetry={props.onRetry}
              showEmpty={hasRecords && !props.tasks.historyLoading && !props.tasks.historyError}
            />
          </Tabs.Panel>
        ))}
      </Tabs.Root>
    </section>
  );
}
