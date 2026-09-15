import { useEffect, useRef, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type BrowserLoginInput } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { BrowserSurface } from "@/features/upstreams/components/browser-login/browser-surface";
import { notifyOperationError } from "@/lib/operation-feedback";
import { oauthPollingStatuses, oauthStatusLabels, workbenchKeys } from "../constants";
import { WorkbenchOAuthSMSAttachment } from "./workbench-oauth-sms-attachment";

export function WorkbenchOAuthBatchBrowser(props: { id: string; disabled: boolean }): ReactElement {
  const client = useQueryClient();
  const mounted = useRef(true);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const nextInput = useRef<BrowserLoginInput | null>(null);
  const query = useQuery({
    queryKey: workbenchKeys.oauth(props.id),
    queryFn: (context) => api.workbenchOAuth(props.id, context.signal),
    retry: false,
    gcTime: 0,
    refetchInterval: (value) =>
      !value.state.error && oauthPollingStatuses.has(value.state.data?.status ?? "starting")
        ? 1000
        : false,
  });
  const inputMutation = useMutation({
    gcTime: 0,
    mutationFn: (): Promise<unknown> => {
      const input = nextInput.current;
      nextInput.current = null;
      if (!input || !mounted.current) return Promise.resolve();
      return api.workbenchOAuthInput(props.id, input);
    },
    onError: (error) => notifyOperationError(error, "授权页面操作失败，请重试"),
  });
  const finish = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      await queue.current.catch(() => undefined);
      if (!mounted.current) return;
      await api.finishWorkbenchOAuth(props.id);
      await client.invalidateQueries({ queryKey: workbenchKeys.oauth(props.id) });
    },
    onError: (error) => notifyOperationError(error, "账号授权复核失败，请重试"),
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      nextInput.current = null;
      void client.cancelQueries({ queryKey: workbenchKeys.oauth(props.id) });
      client.removeQueries({ queryKey: workbenchKeys.oauth(props.id) });
    };
  }, [client, props.id]);
  const session = query.data;
  const waiting =
    session?.status === "waiting" && !finish.isPending && !query.isError && !props.disabled;
  function send(input: BrowserLoginInput): Promise<unknown> {
    if (!waiting) return Promise.resolve();
    const next = queue.current
      .catch(() => undefined)
      .then(async () => {
        if (!mounted.current) return;
        nextInput.current = input;
        return inputMutation.mutateAsync();
      });
    queue.current = next;
    return next;
  }
  return (
    <section aria-label="当前批量授权页面" className="min-w-0 space-y-3">
      {query.isPending && <ContentLoading label="正在读取当前账号授权页面" />}
      {query.isError && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {session && (
        <p role="status" className="text-sm wrap-anywhere">
          {session.message || oauthStatusLabels[session.status]}
        </p>
      )}
      {session?.image && oauthPollingStatuses.has(session.status) && (
        <BrowserSurface session={session} disabled={!waiting} onInput={send} />
      )}
      {session && oauthPollingStatuses.has(session.status) && (
        <div className="flex flex-wrap justify-end gap-2">
          <WorkbenchOAuthSMSAttachment id={session.id} disabled={!waiting} />
          <Button disabled={!waiting} onClick={() => finish.mutate()}>
            {finish.isPending ? "正在验证授权…" : "登录完成，验证授权"}
          </Button>
        </div>
      )}
    </section>
  );
}
