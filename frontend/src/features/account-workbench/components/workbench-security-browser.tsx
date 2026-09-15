import { useEffect, useRef, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type BrowserInput, type WorkbenchSecuritySession } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { BrowserSurface } from "@/components/browser-surface/browser-surface";
import { notifyOperationError } from "@/lib/operation-feedback";
import { securityActiveStatuses } from "../hooks/use-workbench-security";

export function WorkbenchSecurityBrowser(props: { id: string; disabled?: boolean }): ReactElement {
  const client = useQueryClient();
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const active = useRef(true);
  const inputPayload = useRef<BrowserInput | null>(null);
  const query = useQuery({
    queryKey: ["account-workbench", "security-child", props.id],
    queryFn: (context) => api.workbenchSecurity(props.id, context.signal),
    gcTime: 0,
    retry: false,
    refetchInterval: (value) =>
      securityActiveStatuses.has(value.state.data?.status ?? "starting") && !value.state.error
        ? 1000
        : false,
  });
  const input = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const payload = inputPayload.current;
      inputPayload.current = null;
      if (payload) await api.workbenchSecurityInput(props.id, payload);
    },
    onError: (error) => notifyOperationError(error, "安全验证页面操作失败，请刷新后重试"),
  });
  const next = useMutation({
    mutationFn: async () => {
      await queue.current.catch(() => undefined);
      if (!active.current) return;
      await api.continueWorkbenchSecurity(props.id);
      if (!active.current) return;
      client.setQueryData<WorkbenchSecuritySession>(
        ["account-workbench", "security-child", props.id],
        (old) => (old ? { ...old, status: "running", image: undefined } : old),
      );
      await client.invalidateQueries({
        queryKey: ["account-workbench", "security-child", props.id],
      });
    },
    onError: (error) => notifyOperationError(error, "安全步骤尚未开始，请完成官方验证后重试"),
  });
  const confirm = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const value = query.data;
      if (!active.current || value?.status !== "awaiting_confirmation" || !value.user_id) return;
      await api.confirmWorkbenchSecurityIdentity(props.id, {
        email: value.email,
        user_id: value.user_id,
        confirmed: true,
      });
      await client.invalidateQueries({
        queryKey: ["account-workbench", "security-child", props.id],
      });
    },
    onError: (error) => notifyOperationError(error, "官方身份确认失败，请重新核对当前账号"),
  });
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      inputPayload.current = null;
      void client.cancelQueries({ queryKey: ["account-workbench", "security-child", props.id] });
      client.removeQueries({ queryKey: ["account-workbench", "security-child", props.id] });
    };
  }, [client, props.id]);
  const session = query.data;
  const waiting =
    session?.status === "waiting" && !query.isError && !next.isPending && !props.disabled;
  return (
    <section aria-label="当前账号安全验证" className="grid min-w-0 gap-3">
      {!session && !query.isError ? <ContentLoading label="正在读取当前安全任务" /> : null}
      {session ? (
        <p role="status" className="text-sm wrap-anywhere">
          {session.email}：{session.message}
        </p>
      ) : null}
      {session?.image && securityActiveStatuses.has(session.status) ? (
        <BrowserSurface
          session={session}
          disabled={!waiting}
          onInput={(payload) => {
            if (!waiting) return Promise.resolve();
            const task = queue.current
              .catch(() => undefined)
              .then(() => {
                if (!active.current) return;
                inputPayload.current = payload;
                return input.mutateAsync();
              });
            queue.current = task;
            return task;
          }}
        />
      ) : null}
      {query.isError ? (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}
      {session?.status === "awaiting_confirmation" ? (
        <section
          aria-label="确认当前官方账号身份"
          className="grid min-w-0 gap-2 border-t pt-3 text-sm wrap-anywhere"
        >
          <p>官方邮箱：{session.email}</p>
          <p>官方用户 ID：{session.user_id}</p>
          <Button
            disabled={query.isError || props.disabled || confirm.isPending || !session.user_id}
            onClick={() => confirm.mutate()}
          >
            {confirm.isPending ? "正在确认身份" : "确认当前官方账号"}
          </Button>
        </section>
      ) : null}
      {session && securityActiveStatuses.has(session.status) ? (
        <div className="flex justify-end">
          <Button disabled={!waiting} onClick={() => next.mutate()}>
            验证完成，继续当前账号
          </Button>
        </div>
      ) : null}
    </section>
  );
}
