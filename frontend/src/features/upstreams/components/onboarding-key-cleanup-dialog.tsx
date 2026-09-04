import { Check, Trash2, XCircle } from "lucide-react";

import type { KeyCleanupPreview, Task } from "@/api";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { RefreshButton } from "@/components/refresh-button";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";
import { Badge } from "@/components/ui/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { operationErrorMessage } from "@/lib/operation-feedback";

type CleanupResultItem = {
  keyId: string;
  name: string;
  status: "deleted" | "skipped" | "failed";
  reason: string | null;
};

export function keyCleanupResultItems(task: Task): CleanupResultItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.flatMap((raw) => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const item = raw as Record<string, unknown>;
    const rawStatus = String(item.status ?? "");
    if (
      !(["deleted", "skipped", "failed"] as const).includes(
        rawStatus as CleanupResultItem["status"],
      )
    ) {
      return [];
    }
    return [
      {
        keyId: String(item.key_id ?? ""),
        name: String(item.name ?? ""),
        status: rawStatus as CleanupResultItem["status"],
        reason:
          typeof item.reason === "string" && item.reason.trim() !== "" ? item.reason.trim() : null,
      },
    ];
  });
}

function resultCount(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
}

function cleanupStatusLabel(status: CleanupResultItem["status"]): string {
  if (status === "deleted") return "已删除";
  if (status === "skipped") return "已跳过";
  return "失败";
}

function cleanupStatusVariant(
  status: CleanupResultItem["status"],
): "outline" | "warning" | "destructive" {
  if (status === "deleted") return "outline";
  if (status === "skipped") return "warning";
  return "destructive";
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
  const hasResultRows = task ? keyCleanupResultItems(task).length > 0 : false;
  return hasPreviewRows || hasResultRows ? "wide" : "medium";
}

export function OnboardingKeyCleanupDialogContent(props: OnboardingKeyCleanupDialogProps) {
  const keys = props.preview?.keys ?? [];
  const taskRunning = keyCleanupTaskRunning(props.task);
  const taskFinished = Boolean(props.task && !taskRunning);
  const resultItems = props.task ? keyCleanupResultItems(props.task) : [];
  const controlsDisabled = props.previewPending || props.taskPending || taskRunning;

  return (
    <div data-slot="onboarding-key-cleanup-dialog" className="contents">
      <DialogHeader>
        <DialogTitle>清理无绑定上游 Key</DialogTitle>
        <DialogDescription>
          只清理上游实时存在、但未与 Console 账号绑定且不在开户待续中的 Key。
        </DialogDescription>
      </DialogHeader>
      <DialogBody className="min-h-0 overflow-auto pr-0">
        {props.previewPending ? <TaskStartupState message="正在扫描上游 Key 与绑定关系" /> : null}
        {props.previewError ? (
          <p className="text-destructive break-words text-sm" role="alert">
            {operationErrorMessage(props.previewError, "无绑定 Key 扫描失败")}
          </p>
        ) : null}
        {!props.task && props.preview ? (
          <div className="grid gap-4">
            {keys.length > 0 ? (
              <div className="border-destructive/40 bg-destructive/5 rounded-lg border px-4 py-3 text-sm leading-6">
                将永久删除下列 {keys.length} 个上游
                Key。执行前后端会再次复核绑定关系，已建立绑定或进入开户待续的 Key 会自动跳过。
              </div>
            ) : (
              <div className="text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-center text-sm">
                未发现无绑定的上游 Key。
              </div>
            )}
            {keys.length > 0 ? (
              <DataTablePanel>
                <Table
                  className="min-w-[680px] table-fixed"
                  containerClassName="max-h-[24rem] overflow-auto"
                >
                  <TableHeader className="sticky top-0 z-10">
                    <TableRow>
                      <TableHead className="w-[30%]">Key 名称</TableHead>
                      <TableHead className="w-[28%]">Key ID</TableHead>
                      <TableHead className="w-[24%]">上游分组</TableHead>
                      <TableHead className="w-[18%]">状态</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {keys.map((key) => (
                      <TableRow key={key.key_id}>
                        <TableCell className="font-medium">
                          {key.name || `Key ${key.key_id}`}
                        </TableCell>
                        <TableCell>{key.key_id}</TableCell>
                        <TableCell>{key.group_id || "未返回"}</TableCell>
                        <TableCell>{key.status || "未返回"}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </DataTablePanel>
            ) : null}
          </div>
        ) : null}
        {props.taskPending && !props.task ? (
          <TaskStartupState message="正在创建 Key 清理任务" />
        ) : null}
        {props.taskError ? (
          <p className="text-destructive break-words text-sm" role="alert">
            {operationErrorMessage(props.taskError, "Key 清理任务启动或读取失败")}
          </p>
        ) : null}
        {props.task && taskRunning ? (
          <TaskProgressState message={props.task.message} progress={props.task.progress} />
        ) : null}
        {props.task && taskFinished ? (
          <div className="grid gap-4">
            <div className="grid grid-cols-3 divide-x rounded-lg border text-center">
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
            <p className={props.task.status === "failed" ? "text-destructive text-sm" : "text-sm"}>
              {props.task.message}
            </p>
            {resultItems.length > 0 ? (
              <DataTablePanel>
                <Table
                  className="min-w-[680px] table-fixed"
                  containerClassName="max-h-[22rem] overflow-auto"
                >
                  <TableHeader className="sticky top-0 z-10">
                    <TableRow>
                      <TableHead className="w-[30%]">Key 名称</TableHead>
                      <TableHead className="w-[28%]">Key ID</TableHead>
                      <TableHead className="w-[16%]">结果</TableHead>
                      <TableHead className="w-[26%]">说明</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {resultItems.map((item) => (
                      <TableRow key={item.keyId}>
                        <TableCell className="font-medium">
                          {item.name || `Key ${item.keyId}`}
                        </TableCell>
                        <TableCell>{item.keyId}</TableCell>
                        <TableCell>
                          <Badge variant={cleanupStatusVariant(item.status)}>
                            {cleanupStatusLabel(item.status)}
                          </Badge>
                        </TableCell>
                        <TableCell>{item.reason || "—"}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </DataTablePanel>
            ) : null}
          </div>
        ) : null}
      </DialogBody>
      <DialogFooter>
        {!props.task ? (
          <>
            <Button
              variant="outline"
              disabled={controlsDisabled}
              onClick={() => props.onOpenChange(false)}
            >
              {keys.length > 0 ? "取消" : "关闭"}
            </Button>
            <RefreshButton
              pending={props.previewPending}
              disabled={controlsDisabled}
              ariaLabel="刷新扫描结果"
              onClick={props.onRefresh}
            />
            {keys.length > 0 ? (
              <Button variant="destructive" disabled={controlsDisabled} onClick={props.onConfirm}>
                <Trash2 aria-hidden="true" />
                确认删除 {keys.length} 个 Key
              </Button>
            ) : null}
          </>
        ) : taskFinished ? (
          <Button onClick={props.onComplete}>
            {props.task?.status === "failed" ? (
              <XCircle aria-hidden="true" />
            ) : (
              <Check aria-hidden="true" />
            )}
            完成
          </Button>
        ) : (
          <Button variant="outline" disabled>
            清理进行中
          </Button>
        )}
      </DialogFooter>
    </div>
  );
}

export function OnboardingKeyCleanupDialog(props: OnboardingKeyCleanupDialogProps) {
  const controlsDisabled =
    props.previewPending || props.taskPending || keyCleanupTaskRunning(props.task);
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
