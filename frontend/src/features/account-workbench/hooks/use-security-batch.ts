import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type WorkbenchSecurityBatch,
  type WorkbenchSecurityBatchInput,
  type WorkbenchSecurityBatchPreview,
} from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";

const securityBatchKey = (id: string | null): readonly [string, string, string | null] => [
  "account-workbench",
  "security-batch",
  id,
];
export type SecurityBatchControl = {
  preview: WorkbenchSecurityBatchPreview | null;
  batch: WorkbenchSecurityBatch | null;
  parsing: boolean;
  starting: boolean;
  queryFailed: boolean;
  retrying: boolean;
  parse: (input: WorkbenchSecurityBatchInput) => void;
  start: () => void;
  close: () => void;
  discard: () => void;
  retry: () => void;
};

export function useSecurityBatch(): SecurityBatchControl {
  const client = useQueryClient();
  const [preview, setPreview] = useState<WorkbenchSecurityBatchPreview | null>(null);
  const [initial, setInitial] = useState<WorkbenchSecurityBatch | null>(null);
  const active = useRef<string | null>(null);
  const previewID = useRef<string | null>(null);
  const input = useRef<WorkbenchSecurityBatchInput | null>(null);
  const generation = useRef(0);
  const query = useQuery({
    queryKey: securityBatchKey(initial?.id ?? null),
    queryFn: (context) => api.workbenchSecurityBatch(initial!.id, context.signal),
    enabled: initial !== null,
    gcTime: 0,
    retry: false,
    refetchInterval: (value) =>
      ["queued", "running"].includes(value.state.data?.status ?? initial?.status ?? "failed") &&
      !value.state.error
        ? 1000
        : false,
  });
  const discard = useCallback((): void => {
    generation.current += 1;
    input.current = null;
    const id = previewID.current;
    previewID.current = null;
    setPreview(null);
    if (id) void api.discardWorkbenchSecurityBatchPreview(id).catch(() => undefined);
  }, []);
  const release = useCallback((): void => {
    discard();
    const id = active.current;
    active.current = null;
    if (id) {
      void api.cancelWorkbenchSecurityBatch(id).catch(() => undefined);
      void client.cancelQueries({ queryKey: securityBatchKey(id) });
      client.removeQueries({ queryKey: securityBatchKey(id) });
    }
  }, [client, discard]);
  const parse = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const requested = generation.current;
      if (!input.current) return null;
      const request = api.previewWorkbenchSecurityBatch(input.current);
      input.current = null;
      const value = await request;
      if (requested !== generation.current) {
        if (value.id) await api.discardWorkbenchSecurityBatchPreview(value.id);
        return null;
      }
      return value;
    },
    onSuccess: (value) => {
      if (value) {
        previewID.current = value.id;
        setPreview(value);
      }
    },
    onError: (error) => notifyOperationError(error, "批量安全操作预览失败，请核对账号后重试"),
  });
  const start = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const requested = generation.current,
        id = previewID.current;
      if (!id) return null;
      const value = await api.startWorkbenchSecurityBatch(id);
      if (requested !== generation.current) {
        await api.cancelWorkbenchSecurityBatch(value.id);
        return null;
      }
      return value;
    },
    onSuccess: (value) => {
      if (!value) return;
      previewID.current = null;
      setPreview(null);
      active.current = value.id;
      setInitial(value);
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "批量安全任务未启动，请核对预览后重试"),
  });
  useEffect(() => {
    window.addEventListener("pagehide", release);
    return () => {
      window.removeEventListener("pagehide", release);
      release();
    };
  }, [release]);
  const expiry = initial?.expires_at ?? preview?.expires_at;
  useEffect(() => {
    if (!expiry) return;
    const remaining = Date.parse(expiry) - Date.now();
    const timer = setTimeout(
      () => {
        release();
        setInitial(null);
      },
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [expiry, release]);
  return {
    preview,
    batch: query.data ?? initial,
    parsing: parse.isPending,
    starting: start.isPending,
    queryFailed: query.isError,
    retrying: query.isFetching,
    parse: (value) => {
      discard();
      input.current = value;
      parse.mutate();
    },
    start: () => start.mutate(),
    close: () => {
      release();
      setInitial(null);
      parse.reset();
      start.reset();
    },
    discard: () => {
      discard();
      parse.reset();
    },
    retry: () => {
      void query.refetch();
    },
  };
}
