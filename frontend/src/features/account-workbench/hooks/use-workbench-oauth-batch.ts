import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type WorkbenchOAuthBatch,
  type WorkbenchOAuthBatchInput,
  type WorkbenchOAuthBatchPreview,
  type WorkbenchReauthorizationInput,
  type WorkbenchQueueRecovery,
  type WorkbenchSourceReauthorizationInput,
} from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";

export function useWorkbenchOAuthBatch(): {
  preview: WorkbenchOAuthBatchPreview | null;
  batch: WorkbenchOAuthBatch | null;
  parsing: boolean;
  starting: boolean;
  cancelling: boolean;
  failed: boolean;
  refreshing: boolean;
  parse: (input: WorkbenchOAuthBatchInput) => void;
  reauthorize: (input: WorkbenchReauthorizationInput) => void;
  reauthorizeSource: (input: WorkbenchSourceReauthorizationInput) => void;
  start: () => void;
  resume: (queue: WorkbenchQueueRecovery) => void;
  closePreview: () => void;
  close: () => void;
  retry: () => void;
} {
  const client = useQueryClient();
  const [preview, setPreview] = useState<WorkbenchOAuthBatchPreview | null>(null);
  const [batch, setBatch] = useState<WorkbenchOAuthBatch | null>(null);
  const [expired, setExpired] = useState(false);
  const input = useRef<
    | { kind: "input"; value: WorkbenchOAuthBatchInput }
    | { kind: "profiles"; value: WorkbenchReauthorizationInput }
    | { kind: "source-profiles"; value: WorkbenchSourceReauthorizationInput }
    | null
  >(null);
  const previewID = useRef<string | null>(null);
  const batchID = useRef<string | null>(null);
  const retainBatch = useRef(false);
  const preservedGenerations = useRef(new Set<number>());
  const generation = useRef(0);
  const mounted = useRef(true);
  const resumeQueue = useRef<WorkbenchQueueRecovery | null>(null);
  const query = useQuery({
    queryKey: workbenchKeys.batch(batch?.id ?? null),
    queryFn: (context) => api.workbenchOAuthBatch(batch!.id, context.signal),
    enabled: !!batch && !expired,
    gcTime: 0,
    retry: false,
    refetchInterval: (value) => {
      const status = value.state.data?.status ?? batch?.status;
      return !value.state.error && (status === "queued" || status === "running") ? 1000 : false;
    },
  });
  const closePreview = useCallback((): void => {
    generation.current += 1;
    input.current = null;
    const id = previewID.current;
    previewID.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchOAuthBatchPreview(id).catch(() => undefined);
  }, []);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const current = generation.current;
      const value = input.current;
      input.current = null;
      if (!value) return null;
      let result: WorkbenchOAuthBatchPreview;
      if (value.kind === "input") result = await api.previewWorkbenchOAuthBatch(value.value);
      else if (value.kind === "source-profiles")
        result = await api.previewWorkbenchSourceReauthorization(value.value);
      else result = await api.previewWorkbenchReauthorization(value.value);
      if (!mounted.current || current !== generation.current) {
        if (result.id) await api.discardWorkbenchOAuthBatchPreview(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (value) => {
      if (!value) return;
      previewID.current = value.id || null;
      setPreview(value);
    },
    onError: (error) => notifyOperationError(error, "批量授权解析失败，请检查输入后重试"),
  });
  const start = useMutation({
    gcTime: 0,
    mutationFn: async (id: string) => {
      const current = generation.current;
      const queue = resumeQueue.current;
      resumeQueue.current = null;
      const value = queue
        ? await api.resumeWorkbenchOAuthQueue(queue)
        : await api.startWorkbenchOAuthBatch(id);
      if (!mounted.current || current !== generation.current) {
        if (!preservedGenerations.current.delete(current) || !value.recovery_enabled)
          await api.cancelWorkbenchOAuthBatch(value.id);
        return null;
      }
      return value;
    },
    onSuccess: (value) => {
      if (!value) return;
      previewID.current = null;
      setPreview(null);
      const previous = batchID.current;
      if (previous && previous !== value.id) {
        void api
          .cancelWorkbenchOAuthBatch(previous)
          .catch((error) => notifyOperationError(error, "原批次结果清理失败，将在到期后清除"));
        client.removeQueries({ queryKey: workbenchKeys.batch(previous) });
      }
      batchID.current = value.id;
      retainBatch.current = value.recovery_enabled === true;
      setBatch(value);
      setExpired(false);
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      void client.invalidateQueries({ queryKey: workbenchKeys.queueRecoveries });
    },
    onError: (error) => {
      closePreview();
      notifyOperationError(error, "批量授权未启动，请重新解析账号");
    },
  });
  const close = useMutation({
    mutationFn: (id: string) => api.cancelWorkbenchOAuthBatch(id),
    onSuccess: () => {
      const id = batchID.current;
      batchID.current = null;
      setBatch(null);
      client.removeQueries({ queryKey: workbenchKeys.batch(id) });
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "结束批量授权失败，请重试"),
  });
  const release = useCallback((): void => {
    generation.current += 1;
    input.current = null;
    resumeQueue.current = null;
    const currentPreview = previewID.current;
    previewID.current = null;
    if (currentPreview)
      void api.discardWorkbenchOAuthBatchPreview(currentPreview).catch(() => undefined);
    const currentBatch = batchID.current;
    batchID.current = null;
    if (currentBatch) {
      if (!retainBatch.current)
        void api.cancelWorkbenchOAuthBatch(currentBatch).catch(() => undefined);
      void client.cancelQueries({ queryKey: workbenchKeys.batch(currentBatch) });
      client.removeQueries({ queryKey: workbenchKeys.batch(currentBatch) });
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
    if (!batch) return;
    const remaining = Date.parse(batch.expires_at) - Date.now();
    const timer = setTimeout(
      () => {
        retainBatch.current = false;
        release();
        setExpired(true);
      },
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [batch, release]);
  const current = query.data ?? batch;
  return {
    preview,
    batch:
      current && expired
        ? {
            ...current,
            available: 0,
            current_oauth_id: undefined,
            status: "cancelled",
            message: "批量授权已到期，请重新解析账号",
          }
        : current,
    parsing: parse.isPending,
    starting: start.isPending,
    cancelling: close.isPending,
    failed: query.isError,
    refreshing: query.isFetching,
    parse: (value) => {
      closePreview();
      input.current = { kind: "input", value };
      parse.mutate();
    },
    reauthorize: (value) => {
      closePreview();
      input.current = { kind: "profiles", value };
      parse.mutate();
    },
    reauthorizeSource: (value) => {
      closePreview();
      input.current = { kind: "source-profiles", value };
      parse.mutate();
    },
    start: () => {
      if (previewID.current) start.mutate(previewID.current);
    },
    resume: (queue) => {
      closePreview();
      resumeQueue.current = queue;
      start.mutate("");
    },
    closePreview,
    close: () => {
      closePreview();
      if (batchID.current) close.mutate(batchID.current);
      else setBatch(null);
    },
    retry: () => void query.refetch(),
  };
}
