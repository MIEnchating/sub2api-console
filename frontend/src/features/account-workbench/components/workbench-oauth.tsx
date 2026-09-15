import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Pause, Save, ShieldCheck } from "lucide-react";
import {
  api,
  type Task,
  type WorkbenchPreview,
  type WorkbenchScope,
  type WorkbenchOAuthCheckpoint,
} from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { BrowserSurface } from "@/components/browser-surface/browser-surface";
import { notifyOperationError } from "@/lib/operation-feedback";
import { oauthPollingStatuses, oauthStatusLabels, workbenchKeys } from "../constants";
import { useWorkbenchOAuth } from "../hooks/use-workbench-oauth";
import type { OAuthPreviewValues } from "../lib/schemas";
import { WorkbenchOAuthOptions } from "./workbench-oauth-options";
import { WorkbenchOAuthStart } from "./workbench-oauth-start";
import { WorkbenchPreviewPanel } from "./workbench-preview";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchOAuthCheckpoints } from "./workbench-oauth-checkpoints";
import { WorkbenchOAuthSecurity } from "./workbench-oauth-security";
import { WorkbenchCheckpointSecurity } from "./workbench-checkpoint-security";
import { WorkbenchOAuthSMSAttachment } from "./workbench-oauth-sms-attachment";
import { WorkbenchSourceProfileCreate } from "./workbench-source-profile-create";

export function WorkbenchOAuth(props: { scope?: WorkbenchScope } = {}): ReactElement {
  const oauth = useWorkbenchOAuth();
  const client = useQueryClient();
  const [preview, setPreview] = useState<WorkbenchPreview | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [converting, setConverting] = useState(false);
  const [suspending, setSuspending] = useState(false);
  const [securityOpen, setSecurityOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [securityCheckpoint, setSecurityCheckpoint] = useState<WorkbenchOAuthCheckpoint | null>(
    null,
  );
  const activePreview = useRef<string | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const clearPreview = useCallback((): void => {
    generation.current += 1;
    const id = activePreview.current;
    activePreview.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
  }, []);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async (options: OAuthPreviewValues) => {
      if (oauth.session?.status !== "authorized") throw new Error("请先完成账号授权");
      const requestedGeneration = generation.current;
      const result = await api.previewWorkbenchOAuth(oauth.session.id, {
        ...options,
        scope: props.scope,
        export_only: props.scope === "local-export" || undefined,
        template_id: options.template_id || undefined,
      });
      if (!mounted.current || requestedGeneration !== generation.current) {
        await api.discardWorkbenchPreview(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (value) => {
      if (!value) return;
      activePreview.current = value.id;
      setPreview(value);
    },
    onError: (error) => notifyOperationError(error, "授权账号预览失败，请重试"),
  });
  const create = useMutation({
    mutationFn: (id: string) => api.importWorkbenchPreview(id),
    onSuccess: (value) => {
      activePreview.current = null;
      clearPreview();
      oauth.close();
      parse.reset();
      setTask(value);
      toast.success("授权账号导入任务已创建");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "导入任务创建失败，请重新预览后重试"),
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current += 1;
      const id = activePreview.current;
      activePreview.current = null;
      if (id) void api.discardWorkbenchPreview(id).catch(() => undefined);
    };
  }, []);
  const session = oauth.session;
  useEffect(() => {
    if (session?.status !== "authorized") {
      setSecurityOpen(false);
      setProfileOpen(false);
    }
  }, [session?.status]);
  const busy =
    parse.isPending ||
    create.isPending ||
    converting ||
    oauth.suspending ||
    securityOpen ||
    profileOpen ||
    securityCheckpoint !== null;
  const waiting =
    session?.status === "waiting" && !oauth.finishing && !oauth.suspending && !oauth.queryFailed;
  const authorized = session?.status === "authorized" && !oauth.queryFailed;
  const active = session && oauthPollingStatuses.has(session.status);
  const close = (): void => {
    setSecurityOpen(false);
    setProfileOpen(false);
    clearPreview();
    parse.reset();
    oauth.close();
  };
  return (
    <div className="grid min-w-0 gap-4">
      <section className="grid min-w-0 gap-4" aria-label="账号授权登录">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">OpenAI 账号授权</h2>
          <WorkbenchOAuthCheckpoints
            scope={props.scope}
            disabled={busy || !!active || !!authorized || oauth.starting}
            onRestore={oauth.restore}
            onSecurity={setSecurityCheckpoint}
            onOAuth={oauth.accept}
          />
        </div>
        {!active && !authorized && !oauth.starting ? (
          <WorkbenchOAuthStart
            disabled={busy}
            retry={oauth.startFailed || session !== null}
            onStart={(input) => {
              clearPreview();
              oauth.start({ ...input, scope: props.scope });
            }}
          />
        ) : null}
        {oauth.starting || session?.status === "starting" ? (
          <ContentLoading label={oauthStatusLabels.starting} />
        ) : null}
        {oauth.finishing || session?.status === "verifying" ? (
          <ContentLoading label={oauthStatusLabels.verifying} />
        ) : null}
        {session && !active ? (
          <p role="status" className="text-sm">
            {oauthStatusLabels[session.status]}
          </p>
        ) : null}
        {session?.status === "waiting" && !oauth.finishing ? (
          <p role="status" className="text-sm wrap-anywhere">
            {session.message || oauthStatusLabels.waiting}
          </p>
        ) : null}
        {active && session.image ? (
          <BrowserSurface
            key={session.id}
            session={session}
            disabled={!waiting}
            onInput={oauth.send}
          />
        ) : null}
        {session?.status === "waiting" && !session.image ? (
          <ContentLoading label="正在读取授权页面" />
        ) : null}
        {oauth.queryFailed ? <ContentRetry pending={oauth.retrying} onRetry={oauth.retry} /> : null}
        {authorized ? (
          <WorkbenchOAuthOptions
            scope={props.scope}
            key={session.id}
            disabled={busy}
            onChange={clearPreview}
            onSubmit={(values) => {
              clearPreview();
              parse.mutate(values);
            }}
          />
        ) : null}
        {parse.isPending ? <ContentLoading label="正在生成授权账号预览" /> : null}
        {oauth.starting || session ? (
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={create.isPending || converting || oauth.suspending || securityOpen}
              onClick={close}
            >
              结束授权
            </Button>
            {authorized ? (
              <Button variant="outline" disabled={busy} onClick={() => setSecurityOpen(true)}>
                <ShieldCheck aria-hidden="true" />
                设置账号安全
              </Button>
            ) : null}
            {authorized && (props.scope ?? session.scope) === "local-export" ? (
              <Button variant="outline" disabled={busy} onClick={() => setProfileOpen(true)}>
                <Save aria-hidden="true" />
                保存本地登录资料
              </Button>
            ) : null}
            {active ? <WorkbenchOAuthSMSAttachment id={session.id} disabled={!waiting} /> : null}
            {active ? (
              <Button variant="outline" disabled={!waiting} onClick={() => setSuspending(true)}>
                <Pause aria-hidden="true" />
                暂停并保存授权
              </Button>
            ) : null}
            {active ? (
              <Button type="button" disabled={!waiting} onClick={oauth.finish}>
                登录完成，验证授权
              </Button>
            ) : null}
          </div>
        ) : null}
      </section>
      {profileOpen && authorized ? (
        <WorkbenchSourceProfileCreate
          source={{ source_oauth_id: session.id }}
          onClose={() => setProfileOpen(false)}
        />
      ) : null}
      {securityCheckpoint ? (
        <WorkbenchCheckpointSecurity
          checkpoint={securityCheckpoint}
          onClose={() => setSecurityCheckpoint(null)}
          onOAuth={(value) => {
            oauth.accept(value);
            setSecurityCheckpoint(null);
          }}
        />
      ) : null}
      {securityOpen && authorized ? (
        <WorkbenchOAuthSecurity sourceId={session.id} onClose={() => setSecurityOpen(false)} />
      ) : null}
      <ConfirmActionDialog
        open={suspending}
        title="暂停并保存授权"
        description="保存当前私有登录状态并停止原授权。恢复期限保持不变，恢复后需人工继续登录；已购短信订单保留，费用与有效期按供应商规则执行。"
        confirmLabel="确认暂停授权"
        pending={oauth.suspending}
        onOpenChange={setSuspending}
        onConfirm={() => {
          void oauth
            .suspend()
            .then(() => setSuspending(false))
            .catch(() => setSuspending(false));
        }}
      />
      {preview ? (
        <WorkbenchPreviewPanel
          preview={preview}
          pending={create.isPending}
          onConversionPendingChange={setConverting}
          onConverted={(value) => {
            activePreview.current = null;
            clearPreview();
            oauth.close();
            parse.reset();
            setTask(value);
          }}
          onConfirm={() => create.mutate(preview.id)}
          onDiscard={clearPreview}
        />
      ) : null}
      {create.isPending ? <TaskStartupState message="正在创建授权账号导入任务" /> : null}
      {task ? <WorkbenchTask task={task} /> : null}
    </div>
  );
}
