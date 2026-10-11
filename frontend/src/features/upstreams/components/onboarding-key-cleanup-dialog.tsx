import { Check, Trash2, XCircle } from "lucide-react";
import { useMemo } from "react";

import type { KeyCleanupPreview, Task } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { RefreshButton } from "@/components/refresh-button";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { QueryErrorToast } from "@/components/query-error-toast";
import { KeyCleanupPreviewTable, KeyCleanupResultsTable } from "./onboarding-key-cleanup-tables";

export type CleanupResultItem = {
  keyId: string;
  name: string;
  status: "deleted" | "skipped" | "failed";
  reason: string | null;
};

function isCleanupResultItem(raw: unknown): raw is Record<string, unknown> {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return false;
  const status = String((raw as Record<string, unknown>).status ?? "");
  return status === "deleted" || status === "skipped" || status === "failed";
}

export function keyCleanupResultItems(task: Task): CleanupResultItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.flatMap((raw) => {
    if (!isCleanupResultItem(raw)) return [];
    return [
      {
        keyId: String(raw.key_id ?? ""),
        name: String(raw.name ?? ""),
        status: String(raw.status) as CleanupResultItem["status"],
        reason:
          typeof raw.reason === "string" && raw.reason.trim() !== "" ? raw.reason.trim() : null,
      },
    ];
  });
}

function resultCount(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
}

export type OnboardingKeyCleanupDialogProps = {
  open: boolean;
  preview: KeyCleanupPreview | null;
  previewPending: boolean;
  previewError: unknown;
  task: Task | null;
  taskPending: boolean;
  taskError: unknown;
  onOpenChange: (open: boolean) => void;
  onRefresh: () => void;
  onConfirm: () => void;
  onComplete: () => void;
};

function keyCleanupTaskRunning(task: Task | null): boolean {
  return Boolean(task && ["queued", "running", "waiting_input"].includes(task.status));
}

export const keyCleanupDialogLayout = {
  height: "adaptive",
  content: "grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
} as const;

export function keyCleanupDialogWidth(
  preview: KeyCleanupPreview | null,
  task: Task | null,
): "medium" | "wide" {
  const hasPreviewRows = (preview?.keys.length ?? 0) > 0;
  const hasResultRows =
    Array.isArray(task?.result.items) && task.result.items.some(isCleanupResultItem);
  return hasPreviewRows || hasResultRows ? "wide" : "medium";
}

export function OnboardingKeyCleanupDialogContent(props: OnboardingKeyCleanupDialogProps) {
  const keys = props.preview?.keys ?? [];
  const taskRunning = keyCleanupTaskRunning(props.task);
  const taskFinished = Boolean(props.task && !taskRunning);
  const resultItems = useMemo(
    () => (props.task ? keyCleanupResultItems(props.task) : []),
    [props.task],
  );
  const controlsDisabled = props.previewPending || props.taskPending || taskRunning;
  const closeButtonLabel = keys.length > 0 ? "取消" : "关闭";

  return (
    <div data-slot="onboarding-key-cleanup-dialog" className="contents">
      <DialogHeader>
        <DialogTitle>清理无绑定上游 Key</DialogTitle>
        <DialogDescription>
          只清理上游实时存在、但未与 Console 账号绑定且不在开户待续中的 Key。
        </DialogDescription>
      </DialogHeader>
      <DialogBody className="min-h-0 overflow-auto pr-0">
        {props.previewPending && !props.preview ? (
          <ContentLoading label="正在扫描上游 Key 与绑定关系" />
        ) : null}
        {props.previewError ? (
          <QueryErrorToast error={props.previewError} fallback="无绑定 Key 扫描失败" />
        ) : null}
        {!props.task && props.preview ? (
          <div className="grid gap-4">
            {keys.length > 0 ? (
              <div className="border-destructive/40 bg-destructive/5 rounded-lg border px-4 py-3 text-sm leading-6">
                将永久删除全部 {keys.length} 个上游
                Key。执行前后端会再次复核绑定关系，已建立绑定或进入开户待续的 Key 会自动跳过。
              </div>
            ) : (
              <div className="text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-center text-sm">
                未发现无绑定的上游 Key。
              </div>
            )}
            {keys.length > 0 ? (
              <KeyCleanupPreviewTable key={props.preview.host} keys={keys} />
            ) : null}
          </div>
        ) : null}
        {props.taskPending && !props.task ? (
          <TaskStartupState message="正在创建 Key 清理任务" />
        ) : null}
        {props.taskError ? (
          <QueryErrorToast error={props.taskError} fallback="Key 清理任务启动或读取失败" />
        ) : null}
        {props.task && taskRunning ? (
          <TaskProgressState
            message={props.task.message}
            progress={props.task.progress}
            taskId={props.task.id}
          />
        ) : null}
        {props.task && (taskFinished || resultItems.length > 0) ? (
          <div className="grid gap-4">
            <div
              role="group"
              aria-label="Key 清理统计"
              className="grid grid-cols-3 divide-x rounded-lg border text-center"
            >
              <div className="px-3 py-3">
                <strong className="block tabular-nums">
                  {resultCount(props.task.result.deleted)}
                </strong>
                <span className="text-muted-foreground text-xs">已删除</span>
              </div>
              <div className="px-3 py-3">
                <strong className="block tabular-nums">
                  {resultCount(props.task.result.skipped)}
                </strong>
                <span className="text-muted-foreground text-xs">已跳过</span>
              </div>
              <div className="px-3 py-3">
                <strong className="block tabular-nums">
                  {resultCount(props.task.result.failed)}
                </strong>
                <span className="text-muted-foreground text-xs">失败</span>
              </div>
            </div>
            {taskFinished ? (
              <p
                className={props.task.status === "failed" ? "text-destructive text-sm" : "text-sm"}
              >
                {props.task.message}
              </p>
            ) : null}
            {resultItems.length > 0 ? (
              <KeyCleanupResultsTable key={props.task.id} items={resultItems} />
            ) : null}
          </div>
        ) : null}
      </DialogBody>
      <DialogFooter>
        {!props.task && (
          <>
            <Button
              variant="outline"
              disabled={props.taskPending || taskRunning}
              onClick={() => props.onOpenChange(false)}
            >
              {closeButtonLabel}
            </Button>
            <RefreshButton
              pending={props.previewPending}
              disabled={controlsDisabled}
              ariaLabel="刷新扫描结果"
              onClick={props.onRefresh}
            />
            {keys.length > 0 ? (
              <Button
                variant="destructive"
                disabled={controlsDisabled || Boolean(props.previewError)}
                onClick={props.onConfirm}
              >
                <Trash2 aria-hidden="true" />
                确认删除 {keys.length} 个 Key
              </Button>
            ) : null}
          </>
        )}
        {props.task && taskFinished && (
          <Button onClick={props.onComplete}>
            {props.task?.status === "failed" ? (
              <XCircle aria-hidden="true" />
            ) : (
              <Check aria-hidden="true" />
            )}
            完成
          </Button>
        )}
        {props.task && !taskFinished && (
          <Button variant="outline" disabled>
            清理进行中
          </Button>
        )}
      </DialogFooter>
    </div>
  );
}

export function OnboardingKeyCleanupDialog(props: OnboardingKeyCleanupDialogProps) {
  const controlsDisabled = props.taskPending || keyCleanupTaskRunning(props.task);
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!controlsDisabled) props.onOpenChange(open);
      }}
    >
      <DialogContent
        width={keyCleanupDialogWidth(props.preview, props.task)}
        height={keyCleanupDialogLayout.height}
        className={keyCleanupDialogLayout.content}
      >
        <OnboardingKeyCleanupDialogContent {...props} />
      </DialogContent>
    </Dialog>
  );
}
