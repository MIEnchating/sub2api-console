import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  api,
  type BrowserInput,
  type WorkbenchSecurityInput,
  type WorkbenchSecuritySession,
} from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";

const sessionKey = (id: string | null): readonly [string, string, string | null] => [
  "account-workbench",
  "security",
  id,
];
export const securityActiveStatuses = new Set([
  "starting",
  "waiting",
  "awaiting_confirmation",
  "running",
]);

type WorkbenchSecurityControl = {
  session: WorkbenchSecuritySession | null;
  starting: boolean;
  continuing: boolean;
  confirming: boolean;
  queryFailed: boolean;
  retrying: boolean;
  start: (input: WorkbenchSecurityInput) => void;
  close: () => void;
  retry: () => void;
  next: () => void;
  confirmIdentity: () => void;
  send: (input: BrowserInput) => Promise<unknown>;
};

export function useWorkbenchSecurity(): WorkbenchSecurityControl {
  const client = useQueryClient();
  const [initial, setInitial] = useState<WorkbenchSecuritySession | null>(null);
  const active = useRef<string | null>(null);
  const pending = useRef<WorkbenchSecurityInput | null>(null);
  const generation = useRef(0);
  const reported = useRef<string | null>(null);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const inputPayload = useRef<{ id: string; payload: BrowserInput } | null>(null);
  const query = useQuery({
    queryKey: sessionKey(initial?.id ?? null),
    queryFn: (context) => api.workbenchSecurity(initial!.id, context.signal),
    enabled: initial !== null,
    gcTime: 0,
    retry: false,
    refetchInterval: (value) =>
      securityActiveStatuses.has(value.state.data?.status ?? initial?.status ?? "failed") &&
      !value.state.error
        ? 1000
        : false,
  });
  const session = query.data ?? initial;
  const release = useCallback((): void => {
    generation.current += 1;
    pending.current = null;
    inputPayload.current = null;
    const id = active.current;
    active.current = null;
    if (id) {
      void api.cancelWorkbenchSecurity(id).catch(() => undefined);
      void client.cancelQueries({ queryKey: sessionKey(id) });
      client.removeQueries({ queryKey: sessionKey(id) });
    }
    queue.current = Promise.resolve();
  }, [client]);
  const start = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const requested = generation.current;
      if (!pending.current) return null;
      const request = api.startWorkbenchSecurity(pending.current);
      pending.current = null;
      const result = await request;
      if (requested !== generation.current) {
        await api.cancelWorkbenchSecurity(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (value) => {
      if (value) {
        active.current = value.id;
        setInitial(value);
      }
    },
    onError: (error) => notifyOperationError(error, "安全任务启动失败，请重新确认账号"),
  });
  const input = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const value = inputPayload.current;
      inputPayload.current = null;
      if (value) await api.workbenchSecurityInput(value.id, value.payload);
    },
    onError: (error) => notifyOperationError(error, "官方页面操作失败，请刷新后重试"),
  });
  const next = useMutation({
    mutationFn: async (id: string) => {
      await queue.current.catch(() => undefined);
      if (active.current !== id) return;
      await api.continueWorkbenchSecurity(id);
      if (active.current !== id) return;
      client.setQueryData<WorkbenchSecuritySession>(sessionKey(id), (old) =>
        old ? { ...old, status: "running", image: undefined } : old,
      );
      await client.invalidateQueries({ queryKey: sessionKey(id) });
    },
    onError: (error) => notifyOperationError(error, "安全操作尚未开始，请完成官方验证后重试"),
  });
  const confirm = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      if (
        active.current !== session?.id ||
        session?.status !== "awaiting_confirmation" ||
        !session.user_id
      )
        return;
      await api.confirmWorkbenchSecurityIdentity(session.id, {
        email: session.email,
        user_id: session.user_id,
        confirmed: true,
      });
      await client.invalidateQueries({ queryKey: sessionKey(session.id) });
    },
    onError: (error) => notifyOperationError(error, "官方身份确认失败，请重新核对当前账号"),
  });
  useEffect(() => {
    window.addEventListener("pagehide", release);
    return () => {
      window.removeEventListener("pagehide", release);
      release();
    };
  }, [release]);
  useEffect(() => {
    if (!initial) return;
    const remaining = Date.parse(initial.expires_at) - Date.now();
    const timer = setTimeout(
      () => {
        release();
        setInitial(null);
        toast.info("账号安全会话已到期");
      },
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [initial, release]);
  useEffect(() => {
    if (!session || securityActiveStatuses.has(session.status) || reported.current === session.id)
      return;
    reported.current = session.id;
    void client.invalidateQueries({ queryKey: workbenchKeys.history });
    if (session.status === "succeeded") toast.success(session.message);
    if (session.status === "failed")
      toast.error(session.message || "账号安全操作失败，请核对官方状态");
  }, [client, session]);
  return {
    session,
    starting: start.isPending,
    continuing: next.isPending,
    confirming: confirm.isPending,
    queryFailed: query.isError,
    retrying: query.isFetching,
    start: (value: WorkbenchSecurityInput): void => {
      release();
      setInitial(null);
      reported.current = null;
      pending.current = value;
      start.mutate();
    },
    close: (): void => {
      release();
      setInitial(null);
      start.reset();
      input.reset();
      next.reset();
      confirm.reset();
    },
    retry: (): void => {
      void query.refetch();
    },
    next: (): void => {
      if (active.current && session?.status === "waiting" && !query.isError)
        next.mutate(active.current);
    },
    confirmIdentity: (): void => {
      if (!query.isError && !confirm.isPending) confirm.mutate();
    },
    send: (payload: BrowserInput): Promise<unknown> => {
      const id = active.current;
      if (!id || session?.status !== "waiting" || next.isPending || query.isError)
        return Promise.resolve();
      const task = queue.current
        .catch(() => undefined)
        .then(() => {
          if (active.current !== id) return;
          inputPayload.current = { id, payload };
          return input.mutateAsync();
        });
      queue.current = task;
      return task;
    },
  };
}
