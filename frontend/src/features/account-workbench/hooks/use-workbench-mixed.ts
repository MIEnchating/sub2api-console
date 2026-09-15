import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type WorkbenchRunInput,
  type WorkbenchRunPreview,
  type WorkbenchRunView,
  type WorkbenchQueueRecovery,
} from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";

export function useWorkbenchMixed(): {
  preview: WorkbenchRunPreview | null;
  run: WorkbenchRunView | null;
  parsing: boolean;
  starting: boolean;
  cancelling: boolean;
  failed: boolean;
  refreshing: boolean;
  parse: (input: WorkbenchRunInput) => void;
  start: () => void;
  resume: (queue: WorkbenchQueueRecovery) => void;
  discard: () => void;
  close: () => void;
  retry: () => void;
} {
  const client = useQueryClient();
  const [preview, setPreview] = useState<WorkbenchRunPreview | null>(null);
  const [run, setRun] = useState<WorkbenchRunView | null>(null);
  const [expired, setExpired] = useState(false);
  const input = useRef<WorkbenchRunInput | null>(null);
  const previewID = useRef<string | null>(null);
  const runID = useRef<string | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const resumeQueue = useRef<WorkbenchQueueRecovery | null>(null);
  const recoveryEnabled = useRef(false);
  const preservedGenerations = useRef(new Set<number>());
  const query = useQuery({
    queryKey: workbenchKeys.run(run?.id ?? null),
    queryFn: (context) => api.workbenchRun(run!.id, context.signal),
    enabled: !!run && !expired,
    gcTime: 0,
    retry: false,
    refetchInterval: (value) => {
      const status = value.state.data?.status ?? run?.status;
      return !value.state.error &&
        (status === "queued" || status === "running" || status === "waiting_input")
        ? 1000
        : false;
    },
  });
  const discard = useCallback((): void => {
    generation.current += 1;
    input.current = null;
    resumeQueue.current = null;
    const id = previewID.current;
    previewID.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchRunPreview(id).catch(() => undefined);
  }, []);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const current = generation.current;
      const value = input.current;
      input.current = null;
      if (!value) return null;
      const result = await api.previewWorkbenchRun(value);
      if (!mounted.current || generation.current !== current) {
        if (result.id) await api.discardWorkbenchRunPreview(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (value) => {
      if (value) {
        previewID.current = value.id || null;
        setPreview(value);
      }
    },
    onError: (error) => notifyOperationError(error, "混合运行预览失败，请重新填写并检查账号资料"),
  });
  const start = useMutation({
    gcTime: 0,
    mutationFn: async (id: string) => {
      const current = generation.current;
      const queue = resumeQueue.current;
      resumeQueue.current = null;
      const value = queue
        ? await api.resumeWorkbenchMixedQueue(queue)
        : await api.startWorkbenchRun(id);
      if (!mounted.current || generation.current !== current) {
        if (!preservedGenerations.current.delete(current) || !value.recovery_enabled)
          await api.cancelWorkbenchRun(value.id);
        return null;
      }
      return value;
    },
    onSuccess: (value) => {
      if (value) {
        previewID.current = null;
        setPreview(null);
        runID.current = value.id;
        recoveryEnabled.current = value.recovery_enabled ?? false;
        setRun(value);
        setExpired(false);
        void client.invalidateQueries({ queryKey: workbenchKeys.history });
        void client.invalidateQueries({ queryKey: workbenchKeys.queueRecoveries });
      }
    },
    onError: (error) => {
      discard();
      notifyOperationError(error, "混合运行未启动，请重新填写账号并预览");
    },
  });
  const close = useMutation({
    mutationFn: (id: string) => api.cancelWorkbenchRun(id),
    onSuccess: () => {
      const id = runID.current;
      runID.current = null;
      recoveryEnabled.current = false;
      setRun(null);
      client.removeQueries({ queryKey: workbenchKeys.run(id) });
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      void client.invalidateQueries({ queryKey: workbenchKeys.queueRecoveries });
    },
    onError: (error) => notifyOperationError(error, "结束混合运行失败，请重试"),
  });
  const release = useCallback((): void => {
    generation.current += 1;
    input.current = null;
    resumeQueue.current = null;
    const currentPreview = previewID.current;
    previewID.current = null;
    if (currentPreview) void api.discardWorkbenchRunPreview(currentPreview).catch(() => undefined);
    const currentRun = runID.current;
    runID.current = null;
    if (currentRun) {
      if (!recoveryEnabled.current) void api.cancelWorkbenchRun(currentRun).catch(() => undefined);
      void client.cancelQueries({ queryKey: workbenchKeys.run(currentRun) });
      client.removeQueries({ queryKey: workbenchKeys.run(currentRun) });
    }
  }, [client]);
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
    if (!run) return;
    const remaining = Date.parse(run.expires_at) - Date.now();
    const timer = setTimeout(
      () => {
        recoveryEnabled.current = false;
        release();
        setExpired(true);
      },
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [run, release]);
  const current = query.data ?? run;
  return {
    preview,
    run:
      current && expired
        ? {
            ...current,
            available: 0,
            current_oauth_id: undefined,
            status: "cancelled",
            message: "混合运行已到期，请重新填写资料",
          }
        : current,
    parsing: parse.isPending,
    starting: start.isPending,
    cancelling: close.isPending,
    failed: query.isError,
    refreshing: query.isFetching,
    parse: (value) => {
      discard();
      input.current = value;
      parse.mutate();
    },
    start: () => {
      if (previewID.current) start.mutate(previewID.current);
    },
    resume: (queue) => {
      if (runID.current || start.isPending || parse.isPending || close.isPending) return;
      discard();
      resumeQueue.current = queue;
      start.mutate("");
    },
    discard,
    close: () => {
      discard();
      if (runID.current) close.mutate(runID.current);
      else setRun(null);
    },
    retry: () => void query.refetch(),
  };
}
