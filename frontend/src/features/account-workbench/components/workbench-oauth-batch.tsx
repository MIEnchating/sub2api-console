import { useState, type ReactElement } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ListPlus, RotateCcw, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import type { Task, WorkbenchScope } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { batchStatusLabels, workbenchKeys } from "../constants";
import { useWorkbenchOAuthBatch } from "../hooks/use-workbench-oauth-batch";
import { WorkbenchOAuthBatchForm } from "./workbench-oauth-batch-form";
import { WorkbenchOAuthBatchPreviewPanel } from "./workbench-oauth-batch-preview";
import { WorkbenchOAuthBatchRows } from "./workbench-oauth-batch-rows";
import { WorkbenchOAuthBatchBrowser } from "./workbench-oauth-batch-browser";
import { WorkbenchOAuthBatchImport } from "./workbench-oauth-batch-import";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchLoginProfiles } from "./workbench-login-profiles";
import { WorkbenchQueueRecovery } from "./workbench-queue-recovery";
import { WorkbenchSourceSecurityBatch } from "./workbench-source-security-batch";
import { WorkbenchSourceProfiles } from "./workbench-source-profiles";

export function WorkbenchOAuthBatch(props: {
  scope?: WorkbenchScope;
  reauthorization?: boolean;
  sourceTaskId?: string;
  localProfiles?: boolean;
}): ReactElement {
  const client = useQueryClient();
  const flow = useWorkbenchOAuthBatch();
  const [editor, setEditor] = useState(false);
  const [closing, setClosing] = useState(false);
  const [finalizing, setFinalizing] = useState(false);
  const [task, setTask] = useState<Task | null>(null);
  const [securityOpen, setSecurityOpen] = useState(false);
  const batch = flow.batch;
  return (
    <div className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-medium">
          {props.reauthorization || props.localProfiles ? "登录资料与重新授权" : "批量账号授权"}
        </h2>
        {!props.localProfiles &&
          !props.reauthorization &&
          !props.sourceTaskId &&
          !batch &&
          !flow.preview && (
            <WorkbenchQueueRecovery
              scope={props.scope}
              kind="oauth-batch"
              disabled={flow.parsing || flow.starting}
              onResume={flow.resume}
            />
          )}
        {!props.localProfiles &&
          !props.reauthorization &&
          !props.sourceTaskId &&
          !batch &&
          !flow.preview && (
            <Button disabled={flow.parsing || flow.starting} onClick={() => setEditor(true)}>
              <ListPlus aria-hidden="true" />
              填写批量账号
            </Button>
          )}
      </div>
      {props.sourceTaskId && !batch && !flow.preview && !flow.parsing && !flow.starting && (
        <Button
          onClick={() =>
            flow.reauthorize({
              account_ids: [],
              source_task_id: props.sourceTaskId,
              fresh_login: true,
            })
          }
        >
          <RotateCcw aria-hidden="true" />
          预览历史账号重新授权
        </Button>
      )}
      {props.reauthorization && !batch && !flow.preview && !flow.parsing && !flow.starting && (
        <WorkbenchLoginProfiles
          onAuthorize={(ids) => flow.reauthorize({ account_ids: ids, fresh_login: true })}
        />
      )}
      {props.localProfiles && !batch && !flow.preview && !flow.parsing && !flow.starting ? (
        <WorkbenchSourceProfiles
          onAuthorize={(items) => flow.reauthorizeSource({ items, fresh_login: true })}
        />
      ) : null}
      {editor && (
        <WorkbenchOAuthBatchForm
          onClose={() => setEditor(false)}
          onSubmit={(input) => flow.parse({ ...input, scope: props.scope })}
        />
      )}
      {flow.parsing && (
        <div className="space-y-2">
          <ContentLoading label="正在解析批量授权账号" />
          <Button variant="outline" onClick={flow.closePreview}>
            取消解析
          </Button>
        </div>
      )}
      {flow.preview && (
        <WorkbenchOAuthBatchPreviewPanel
          key={flow.preview.id}
          preview={flow.preview}
          pending={flow.starting}
          onStart={flow.start}
          onClose={flow.closePreview}
          replacingAvailable={batch?.available}
        />
      )}
      {flow.starting && <ContentLoading label="正在启动批量授权" />}
      {batch && (
        <section aria-label="批量授权进度" className="min-w-0 space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p role="status" className="text-sm">
              {batchStatusLabels[batch.status]}
            </p>
            <Button
              variant="outline"
              disabled={flow.cancelling || flow.starting || finalizing || securityOpen}
              onClick={() => setClosing(true)}
            >
              结束批量授权
            </Button>
          </div>
          <p className="text-sm wrap-anywhere">
            {batch.message}；可导入账号：{batch.available}；任务 ID：{batch.task_id}
          </p>
          <WorkbenchOAuthBatchRows items={batch.items} />
          {batch.status === "authorized" && batch.available > 0 ? (
            <Button
              variant="outline"
              disabled={flow.failed || flow.cancelling || finalizing || securityOpen}
              onClick={() => setSecurityOpen(true)}
            >
              <ShieldCheck aria-hidden="true" />
              批量设置已授权账号安全
            </Button>
          ) : null}
          {batch.fresh_login &&
            batch.status !== "queued" &&
            batch.status !== "running" &&
            batch.items.some(
              (item) =>
                (item.account_id || (props.localProfiles && item.profile_id)) &&
                item.status !== "succeeded",
            ) && (
              <div>
                <Button
                  variant="outline"
                  disabled={
                    flow.failed ||
                    flow.cancelling ||
                    flow.parsing ||
                    flow.starting ||
                    !!flow.preview
                  }
                  onClick={() => {
                    if (props.localProfiles)
                      flow.reauthorizeSource({
                        items: batch.items
                          .filter((item) => item.status !== "succeeded" && item.profile_id)
                          .map((item) => ({
                            id: item.profile_id!,
                            revision: item.profile_revision ?? 0,
                          })),
                        failed_batch_id: batch.id,
                        fresh_login: true,
                      });
                    else
                      flow.reauthorize({
                        account_ids: [],
                        failed_batch_id: batch.id,
                        fresh_login: true,
                      });
                  }}
                >
                  <RotateCcw aria-hidden="true" />
                  重新授权失败项
                </Button>
              </div>
            )}
          {flow.failed && <ContentRetry pending={flow.refreshing} onRetry={flow.retry} />}
          {batch.current_oauth_id && (
            <WorkbenchOAuthBatchBrowser
              key={batch.current_oauth_id}
              id={batch.current_oauth_id}
              disabled={flow.failed || flow.cancelling}
            />
          )}
          {(batch.status === "queued" || batch.status === "running") && !batch.current_oauth_id && (
            <ContentLoading label="正在准备下一个授权账号" />
          )}
          {batch.status === "authorized" && batch.available > 0 && (
            <WorkbenchOAuthBatchImport
              scope={props.scope}
              key={batch.id}
              id={batch.id}
              onPendingChange={setFinalizing}
              disabled={
                flow.failed ||
                flow.cancelling ||
                flow.parsing ||
                flow.starting ||
                !!flow.preview ||
                securityOpen
              }
              onImported={(value) => {
                setTask(value);
                flow.close();
                toast.success("批量授权账号导入任务已创建");
                void client.invalidateQueries({ queryKey: workbenchKeys.history });
              }}
              onConverted={(value) => {
                setTask(value);
                flow.close();
              }}
            />
          )}
        </section>
      )}
      {securityOpen && batch ? (
        <WorkbenchSourceSecurityBatch
          scope={props.scope ?? batch.scope}
          sources={batch.items
            .filter((row) => row.status === "succeeded")
            .map((row) => ({
              source: { oauth_batch_id: batch.id, index: row.index },
              label: `第 ${row.index + 1} 项：${row.email}`,
            }))}
          onClose={() => setSecurityOpen(false)}
        />
      ) : null}
      {task && <WorkbenchTask task={task} />}
      <ConfirmActionDialog
        open={closing && !!batch}
        title="结束批量授权"
        description="结束后将停止剩余授权，并清除本批尚未导入的授权结果。确定结束？"
        confirmLabel="结束并清除本批"
        pending={flow.cancelling || finalizing}
        onOpenChange={setClosing}
        onConfirm={() => {
          if (finalizing) return;
          flow.close();
          setClosing(false);
        }}
      />
    </div>
  );
}
