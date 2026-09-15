import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  api,
  type WorkbenchSecuritySource,
  type WorkbenchOAuthCheckpoint,
  type WorkbenchOAuthSession,
} from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { WorkbenchSecuritySkeleton } from "./workbench-page-skeletons";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { BrowserSurface } from "@/features/upstreams/components/browser-login/browser-surface";
import { securityActiveStatuses, useWorkbenchSecurity } from "../hooks/use-workbench-security";
import { WorkbenchSecurityForm } from "./workbench-security-form";
import { WorkbenchSecurityProfile } from "./workbench-security-profile";
import { WorkbenchSourceSecurityProfile } from "./workbench-source-security-profile";
import { WorkbenchSecurityAuthorization } from "./workbench-security-authorization";

export function WorkbenchSecurity(
  props: {
    source?: WorkbenchSecuritySource;
    checkpoint?: WorkbenchOAuthCheckpoint;
    onOAuth?: (session: WorkbenchOAuthSession) => void;
    disabled?: boolean;
  } = {},
): ReactElement {
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: api.accounts,
    enabled: !props.source && !props.checkpoint,
  });
  const security = useWorkbenchSecurity();
  const session = security.session;
  const active = session && securityActiveStatuses.has(session.status);
  const waiting = session?.status === "waiting" && !security.continuing && !security.queryFailed;
  if (!props.source && !props.checkpoint && accounts.isPending)
    return <WorkbenchSecuritySkeleton />;
  if (!props.source && !props.checkpoint && !accounts.data)
    return <ContentRetry pending={accounts.isFetching} onRetry={() => void accounts.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      <h2 className="text-base font-medium">账号安全设置</h2>
      {!session && !security.starting ? (
        <WorkbenchSecurityForm
          accounts={accounts.data ?? []}
          source={props.source}
          checkpoint={props.checkpoint}
          disabled={props.disabled ?? false}
          onSubmit={security.start}
        />
      ) : null}
      {security.starting || session?.status === "starting" ? (
        <TaskStartupState message="正在启动账号安全任务" />
      ) : null}
      {session ? (
        <section aria-label="账号安全任务" className="grid min-w-0 gap-3">
          <p className="text-sm wrap-anywhere">
            {session.email}
            {session.account_id ? `（账号 ID ${session.account_id}）` : ""}
          </p>
          {session.status !== "failed" ? (
            <p role="status" className="text-sm wrap-anywhere">
              {session.message}
            </p>
          ) : null}
          {session.status === "running" || security.continuing ? (
            <ContentLoading label="正在复核身份并处理安全步骤" />
          ) : null}
          {active && session.image ? (
            <BrowserSurface
              key={session.id}
              session={session}
              disabled={!waiting}
              onInput={security.send}
            />
          ) : null}
          {waiting && !session.image ? (
            <p className="text-sm text-muted-foreground">
              官方登录页面已离开验证步骤，可以点击继续核对。
            </p>
          ) : null}
          {session.artifact_id ? (
            <p className="text-sm wrap-anywhere">私有结果 ID：{session.artifact_id}</p>
          ) : null}
          {session.status === "awaiting_confirmation" ? (
            <section
              aria-label="确认官方账号身份"
              className="grid min-w-0 gap-2 border-t pt-3 text-sm wrap-anywhere"
            >
              <p>官方邮箱：{session.email}</p>
              <p>官方用户 ID：{session.user_id}</p>
              <Button
                disabled={security.confirming || security.queryFailed || !session.user_id}
                onClick={security.confirmIdentity}
              >
                {security.confirming ? "正在确认身份" : "确认此官方账号并继续"}
              </Button>
            </section>
          ) : null}
          {session.status === "succeeded" && session.source_checkpoint_id && props.onOAuth ? (
            <WorkbenchSecurityAuthorization
              securityId={session.id}
              expiresAt={session.expires_at}
              disabled={security.queryFailed || props.disabled}
              onOAuth={props.onOAuth}
            />
          ) : null}
          {session.status === "succeeded" && session.artifact_id && session.account_id ? (
            <WorkbenchSecurityProfile
              key={session.id}
              accountId={session.account_id}
              securityId={session.id}
            />
          ) : null}
          {session.status === "succeeded" &&
          session.artifact_id &&
          session.scope === "local-export" &&
          session.user_id &&
          session.workspace_id ? (
            <WorkbenchSourceSecurityProfile
              securityId={session.id}
              userId={session.user_id}
              workspaceId={session.workspace_id}
            />
          ) : null}
          {security.queryFailed ? (
            <ContentRetry pending={security.retrying} onRetry={security.retry} />
          ) : null}
        </section>
      ) : null}
      {session || security.starting ? (
        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" variant="outline" onClick={security.close}>
            关闭安全任务
          </Button>
          {active ? (
            <Button type="button" disabled={!waiting} onClick={security.next}>
              验证完成，继续
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
