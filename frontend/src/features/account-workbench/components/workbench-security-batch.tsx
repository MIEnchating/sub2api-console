import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  api,
  type WorkbenchSecurityBatchSource,
  type WorkbenchScope,
  type WorkbenchOAuthSession,
} from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { WorkbenchSecurityBatchSkeleton } from "./workbench-page-skeletons";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { useSecurityBatch } from "../hooks/use-security-batch";
import { WorkbenchSecurityBatchForm } from "./workbench-security-batch-form";
import { WorkbenchSecurityBrowser } from "./workbench-security-browser";
import { WorkbenchSecurityProfile } from "./workbench-security-profile";
import { WorkbenchSourceSecurityProfile } from "./workbench-source-security-profile";
import { securitySourceLabel } from "../lib/security-source-label";
import { WorkbenchSecurityAuthorization } from "./workbench-security-authorization";

export function WorkbenchSecurityBatch(
  props: {
    sources?: Array<{ source: WorkbenchSecurityBatchSource; label: string }>;
    scope?: WorkbenchScope;
    onOAuth?: (session: WorkbenchOAuthSession) => void;
  } = {},
): ReactElement {
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: api.accounts,
    enabled: !props.sources,
  });
  const control = useSecurityBatch();
  const [closing, setClosing] = useState(false);
  const view = control.batch,
    preview = control.preview;
  if (!props.sources && accounts.isPending) return <WorkbenchSecurityBatchSkeleton />;
  if (!props.sources && !accounts.data)
    return <ContentRetry pending={accounts.isFetching} onRetry={() => void accounts.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      {!preview && !view && !control.parsing && !control.starting ? (
        <WorkbenchSecurityBatchForm
          accounts={accounts.data ?? []}
          sources={props.sources}
          scope={props.scope}
          disabled={false}
          onSubmit={control.parse}
        />
      ) : null}
      {control.parsing ? <ContentLoading label="正在核对账号安全操作范围" /> : null}
      {control.starting ? <TaskStartupState message="正在创建批量安全任务" /> : null}
      {preview ? (
        <section aria-label="批量安全操作预览" className="grid gap-3 rounded-md border p-4 text-sm">
          <p className="wrap-anywhere">
            {preview.scope === "local-export"
              ? "范围：本地授权来源"
              : `管理目标：${preview.target}`}
          </p>
          <p>
            操作：{preview.operation === "password" ? "设置密码" : "启用 TOTP 双重验证"}，共{" "}
            {preview.items.length} 个账号
          </p>
          <p className="text-muted-foreground">
            将依次核对官方登录身份并处理所选账号。新凭据保存在服务器私有目录，请妥善备份。
          </p>
          {preview.items.some((row) => row.source && "checkpoint_id" in row.source) ? (
            <p>所选检查点将逐项使用，原授权不能再恢复。每个账号需再次确认官方身份。</p>
          ) : null}
          <ul className="grid max-h-64 gap-2 overflow-auto">
            {preview.items.map((row) => (
              <li key={row.index} className="wrap-anywhere">
                第 {row.index + 1} 项：{row.email || "待确认官方身份"}
                {row.account_id ? `（账号 ID ${row.account_id}）` : ""}
                {row.user_id ? <span className="block">官方用户 ID：{row.user_id}</span> : null}
                {row.source ? (
                  <span className="block text-xs text-muted-foreground">
                    {securitySourceLabel(row.source)}
                  </span>
                ) : null}
              </li>
            ))}
            {preview.errors.map((error) => (
              <li key={error.index}>
                第 {error.index + 1} 项：{error.message}
              </li>
            ))}
          </ul>
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="outline" disabled={control.starting} onClick={control.discard}>
              返回修改范围
            </Button>
            <Button
              disabled={
                !preview.id ||
                preview.errors.length > 0 ||
                preview.items.length === 0 ||
                control.starting
              }
              onClick={control.start}
            >
              确认并执行批量安全操作
            </Button>
          </div>
        </section>
      ) : null}
      {view ? (
        <section aria-label="批量安全操作结果" className="grid min-w-0 gap-3">
          <p role="status" className="text-sm">
            {view.message}，已处理 {view.completed}/{view.items.length}，成功 {view.succeeded}
          </p>
          {control.queryFailed ? (
            <ContentRetry pending={control.retrying} onRetry={control.retry} />
          ) : null}
          {view.current_security_id ? (
            <WorkbenchSecurityBrowser
              key={view.current_security_id}
              id={view.current_security_id}
              disabled={control.queryFailed}
            />
          ) : null}
          <ul className="grid max-h-80 gap-2 overflow-auto rounded-md border p-3 text-sm">
            {view.items.map((row) => (
              <li key={row.index} className="wrap-anywhere">
                {row.email || `第 ${row.index + 1} 项`}
                {row.account_id ? `（ID ${row.account_id}）` : ""}：{row.message}
                {row.artifact_id ? ` · 私有结果 ${row.artifact_id}` : ""}
                {["succeeded", "partial", "failed", "cancelled"].includes(view.status) &&
                row.status === "succeeded" &&
                row.security_id &&
                row.source &&
                "checkpoint_id" in row.source ? (
                  <div className="mt-2">
                    <WorkbenchSecurityAuthorization
                      securityId={row.security_id}
                      expiresAt={view.expires_at}
                      disabled={control.queryFailed || closing}
                      onOAuth={props.onOAuth}
                    />
                  </div>
                ) : null}
                {row.status === "succeeded" && row.artifact_id && row.account_id ? (
                  <div className="mt-2">
                    <WorkbenchSecurityProfile
                      key={`${view.id}:${row.account_id}`}
                      accountId={row.account_id}
                      batchId={view.id}
                    />
                  </div>
                ) : null}
                {view.scope === "local-export" &&
                row.status === "succeeded" &&
                row.security_id &&
                row.workspace_id &&
                row.user_id ? (
                  <WorkbenchSourceSecurityProfile
                    securityId={row.security_id}
                    userId={row.user_id}
                    workspaceId={row.workspace_id}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {view || control.parsing || control.starting ? (
        <div className="flex flex-wrap justify-end gap-2">
          {closing ? (
            <>
              <span className="text-sm">结束本批并停止剩余账号？</span>
              <Button variant="outline" onClick={() => setClosing(false)}>
                继续处理
              </Button>
              <Button
                onClick={() => {
                  control.close();
                  setClosing(false);
                }}
              >
                确认结束本批
              </Button>
            </>
          ) : (
            <Button
              variant="outline"
              onClick={() => {
                if (view) setClosing(true);
                else control.close();
              }}
            >
              结束批量安全设置
            </Button>
          )}
        </div>
      ) : null}
    </div>
  );
}
