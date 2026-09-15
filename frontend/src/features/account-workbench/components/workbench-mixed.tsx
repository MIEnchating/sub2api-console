import { useState, type ReactElement } from "react";
import type { Task, WorkbenchScope } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { TaskStartupState } from "@/components/task-startup-state";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { useWorkbenchMixed } from "../hooks/use-workbench-mixed";
import { WorkbenchMixedForm } from "./workbench-mixed-form";
import { WorkbenchMixedPreview } from "./workbench-mixed-preview";
import { WorkbenchMixedRows } from "./workbench-mixed-rows";
import { WorkbenchMixedResult } from "./workbench-mixed-result";
import { WorkbenchOAuthBatchBrowser } from "./workbench-oauth-batch-browser";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchQueueRecovery } from "./workbench-queue-recovery";

export function WorkbenchMixed(props: { scope?: WorkbenchScope } = {}): ReactElement {
  const flow = useWorkbenchMixed();
  const [closing, setClosing] = useState(false);
  const [finalizing, setFinalizing] = useState(false);
  const [task, setTask] = useState<Task | null>(null);
  const run = flow.run;
  return (
    <div className="grid min-w-0 gap-4">
      <div className="flex flex-wrap gap-2">
        <WorkbenchQueueRecovery
          kind="mixed"
          scope={props.scope}
          disabled={flow.parsing || flow.starting || !!flow.preview || !!run}
          onResume={flow.resume}
        />
      </div>
      {!flow.preview && !run && !flow.parsing && !flow.starting && (
        <WorkbenchMixedForm scope={props.scope} onSubmit={flow.parse} />
      )}
      {flow.parsing && (
        <div className="grid gap-2">
          <ContentLoading label="正在解析混合运行范围" />
          <div>
            <Button variant="outline" onClick={flow.discard}>
              取消解析
            </Button>
          </div>
        </div>
      )}
      {flow.preview && (
        <WorkbenchMixedPreview
          key={flow.preview.id}
          preview={flow.preview}
          pending={flow.starting}
          onStart={flow.start}
          onClose={flow.discard}
        />
      )}
      {flow.starting && <TaskStartupState message="正在创建混合运行任务" />}
      {run && (
        <section aria-label="混合运行进度" className="grid min-w-0 gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-medium">混合运行进度</h2>
            <Button
              variant="outline"
              disabled={flow.cancelling || finalizing}
              onClick={() => setClosing(true)}
            >
              结束混合运行
            </Button>
          </div>
          <p role="status" className="text-sm wrap-anywhere">
            {run.message}；可用账号 {run.available} 个；任务 ID：{run.task_id}
          </p>
          <WorkbenchMixedRows rows={run.items} />
          {run.errors.length > 0 && (
            <ul aria-label="混合运行问题" className="max-h-48 overflow-auto text-sm">
              {run.errors.map((error) => (
                <li key={`${error.index}-${error.message}`}>
                  第 {error.index + 1} 项：{error.message}
                </li>
              ))}
            </ul>
          )}
          {flow.failed && <ContentRetry pending={flow.refreshing} onRetry={flow.retry} />}
          {run.current_oauth_id && (
            <WorkbenchOAuthBatchBrowser
              key={run.current_oauth_id}
              id={run.current_oauth_id}
              disabled={flow.failed || flow.cancelling}
            />
          )}
          {(run.status === "running" ||
            run.status === "queued" ||
            run.status === "waiting_input") &&
            !run.current_oauth_id && <ContentLoading label="正在准备账号资料" />}
          {run.status === "ready" && run.available > 0 && run.errors.length === 0 && (
            <WorkbenchMixedResult
              key={run.id}
              id={run.id}
              exportOnly={run.export_only}
              disabled={flow.failed || flow.cancelling}
              onBusy={setFinalizing}
              onTask={(value) => {
                setTask(value);
                setFinalizing(false);
                flow.close();
              }}
            />
          )}
        </section>
      )}
      {task && <WorkbenchTask task={task} />}
      <ConfirmActionDialog
        open={closing && !!run}
        title="结束混合运行"
        description="将停止剩余授权并清除本批尚未导入或转换的账号结果，已创建的导入和私有转换任务仍继续执行。"
        confirmLabel="结束并清除混合结果"
        pending={flow.cancelling}
        onOpenChange={setClosing}
        onConfirm={() => {
          flow.close();
          setClosing(false);
        }}
      />
    </div>
  );
}
