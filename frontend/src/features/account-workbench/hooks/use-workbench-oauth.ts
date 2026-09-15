import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  api,
  type BrowserLoginInput,
  type WorkbenchOAuthSession,
  type WorkbenchOAuthStartInput,
  type WorkbenchOAuthCheckpoint,
} from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { oauthPollingStatuses, workbenchKeys } from "../constants";

type WorkbenchOAuthControl = {
  session: WorkbenchOAuthSession | null;
  starting: boolean;
  startFailed: boolean;
  finishing: boolean;
  queryFailed: boolean;
  retrying: boolean;
  suspending: boolean;
  suspend: () => Promise<void>;
  restore: (checkpoint: WorkbenchOAuthCheckpoint) => void;
  accept: (session: WorkbenchOAuthSession) => void;
  start: (input?: WorkbenchOAuthStartInput) => void;
  close: () => void;
  finish: () => void;
  retry: () => void;
  send: (payload: BrowserLoginInput) => Promise<unknown>;
};

export function useWorkbenchOAuth(): WorkbenchOAuthControl {
  const client = useQueryClient();
  const [session, setSession] = useState<WorkbenchOAuthSession | null>(null);
  const [expiredID, setExpiredID] = useState<string | null>(null);
  const activeID = useRef<string | null>(null);
  const retainSession = useRef(false);
  const preservedGenerations = useRef(new Set<number>());
  const generation = useRef(0);
  const mounted = useRef(true);
  const reported = useRef<string | null>(null);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const startInput = useRef<WorkbenchOAuthStartInput | null>(null);
  const restoreInput = useRef<WorkbenchOAuthCheckpoint | null>(null);
  const query = useQuery({
    queryKey: workbenchKeys.oauth(session?.id ?? null),
    queryFn: (context) => api.workbenchOAuth(session!.id, context.signal),
    enabled: session !== null && expiredID !== session.id,
    gcTime: 0,
    retry: false,
    refetchInterval: (value) =>
      oauthPollingStatuses.has(value.state.data?.status ?? session?.status ?? "failed") &&
      !value.state.error
        ? 1000
        : false,
  });
  const current = query.data ?? session;
  let value = current;
  if (current && current.id === expiredID)
    value = { ...current, status: "expired", image: undefined };
  const start = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const requestedGeneration = generation.current;
      const recovery = restoreInput.current;
      const pending = recovery
        ? api.restoreWorkbenchOAuth(recovery)
        : api.startWorkbenchOAuth(startInput.current ?? {});
      startInput.current = null;
      restoreInput.current = null;
      const result = await pending;
      if (!mounted.current || generation.current !== requestedGeneration) {
        if (!preservedGenerations.current.delete(requestedGeneration) || !result.recovery_enabled)
          await api.cancelWorkbenchOAuth(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (result) => {
      if (!result) return;
      activeID.current = result.id;
      retainSession.current = result.recovery_enabled === true;
      setSession(result);
      void client.invalidateQueries({ queryKey: workbenchKeys.checkpoints });
    },
    onError: (error) => notifyOperationError(error, "授权浏览器启动失败，请重试"),
  });
  const input = useMutation({
    gcTime: 0,
    mutationFn: (request: { id: string; input: BrowserLoginInput }) =>
      api.workbenchOAuthInput(request.id, request.input),
    onError: (error) => notifyOperationError(error, "授权页面操作失败，请重试"),
  });
  const finish = useMutation({
    gcTime: 0,
    mutationFn: async (id: string) => {
      await queue.current;
      if (activeID.current !== id) return;
      await api.finishWorkbenchOAuth(id);
      if (activeID.current !== id) return;
      client.setQueryData<WorkbenchOAuthSession>(workbenchKeys.oauth(id), (previous) =>
        previous ? { ...previous, status: "verifying" } : previous,
      );
      await client.invalidateQueries({ queryKey: workbenchKeys.oauth(id) });
    },
    onError: (error) => notifyOperationError(error, "授权复核未开始，请完成登录后重试"),
  });
  const release = useCallback((): void => {
    generation.current += 1;
    startInput.current = null;
    restoreInput.current = null;
    const id = activeID.current;
    activeID.current = null;
    if (id) {
      if (!retainSession.current) void api.cancelWorkbenchOAuth(id).catch(() => undefined);
      void client.cancelQueries({ queryKey: workbenchKeys.oauth(id) });
      client.removeQueries({ queryKey: workbenchKeys.oauth(id) });
    }
    queue.current = Promise.resolve();
  }, [client]);
  const close = (): void => {
    retainSession.current = false;
    release();
    setSession(null);
    start.reset();
    input.reset();
    finish.reset();
  };
  const suspend = useMutation({
    gcTime: 0,
    mutationFn: async (id: string) => {
      await queue.current;
      if (activeID.current !== id) throw new Error("授权会话已变化，请重新读取");
      await api.suspendWorkbenchOAuth(id);
      if (activeID.current !== id) return;
      activeID.current = null;
      generation.current += 1;
      await client.cancelQueries({ queryKey: workbenchKeys.oauth(id) });
      client.removeQueries({ queryKey: workbenchKeys.oauth(id) });
      setSession(null);
      toast.success("授权已暂停，恢复期限保持不变");
      void client.invalidateQueries({ queryKey: workbenchKeys.checkpoints });
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      void client.invalidateQueries({ queryKey: workbenchKeys.smsReceipts });
    },
    onError: (error) => notifyOperationError(error, "暂停授权失败，请核对当前登录状态"),
  });
  useEffect(() => {
    mounted.current = true;
    const leave = (): void => {
      preservedGenerations.current.add(generation.current);
      release();
    };
    window.addEventListener("pagehide", leave);
    return () => {
      mounted.current = false;
      window.removeEventListener("pagehide", leave);
      leave();
    };
  }, [release]);
  useEffect(() => {
    if (!session) return;
    const remaining = Date.parse(session.expires_at) - Date.now();
    const timer = setTimeout(
      () => {
        setExpiredID(session.id);
        retainSession.current = false;
        release();
      },
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [session, release]);
  useEffect(() => {
    if (!value || oauthPollingStatuses.has(value.status) || reported.current === value.id) return;
    reported.current = value.id;
    if (value.status === "authorized") {
      toast.success("账号授权成功");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    } else if (value.status === "failed") {
      toast.error(value.message || "授权失败，请重新登录");
    }
  }, [value, client]);
  return {
    session: value,
    starting: start.isPending,
    startFailed: start.isError,
    finishing: finish.isPending,
    queryFailed: query.isError,
    retrying: query.isFetching,
    suspending: suspend.isPending,
    suspend: async () => {
      if (value?.status === "waiting" && activeID.current && !finish.isPending && !query.isError)
        await suspend.mutateAsync(activeID.current);
    },
    restore: (checkpoint) => {
      close();
      restoreInput.current = checkpoint;
      reported.current = null;
      start.mutate();
    },
    accept: (result) => {
      close();
      activeID.current = result.id;
      retainSession.current = result.recovery_enabled === true;
      reported.current = null;
      setSession(result);
    },
    start: (input = {}) => {
      close();
      startInput.current = input;
      reported.current = null;
      start.mutate();
    },
    close,
    finish: () => {
      if (value?.status === "waiting" && activeID.current && !query.isError && !suspend.isPending)
        finish.mutate(activeID.current);
    },
    retry: () => void query.refetch(),
    send: (payload) => {
      const id = activeID.current;
      if (value?.status !== "waiting" || finish.isPending || suspend.isPending || query.isError)
        return Promise.resolve();
      const next = queue.current
        .catch(() => undefined)
        .then(() => {
          if (!id || id !== activeID.current) return;
          return input.mutateAsync({ id, input: payload });
        });
      queue.current = next;
      return next;
    },
  };
}
