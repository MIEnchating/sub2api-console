import { useMutation } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { api, type GroupStatus } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { groupBatchActions, type GroupBatchAction } from "../constants";

type GroupTarget = { id: string; name: string };
type BatchRequest = { action: GroupBatchAction; targets: GroupTarget[] };

export function useGroupBatchActions(invalidate: () => Promise<unknown>) {
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(() => new Set());
  const [request, setRequest] = useState<BatchRequest | null>(null);
  const [failed, setFailed] = useState(false);
  const submitting = useRef(false);
  const mutation = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: async (value: BatchRequest) => {
      await api.batchGroupPolicy(
        value.targets.map((target) => target.id),
        value.action,
      );
      return { succeeded: value.targets.length };
    },
    onSuccess: async (result, value) => {
      setFailed(false);
      setSelectedIDs(new Set());
      void invalidate().catch((error: unknown) =>
        notifyOperationError(error, "分组已保存，刷新失败，请刷新列表"),
      );
      setRequest(null);
      toast.success(`${result.succeeded} 个分组已${groupBatchActions[value.action].label}`);
    },
    onError: (error, value) => {
      setFailed(true);
      notifyOperationError(error, "批量操作未获成功确认，请核对分组状态后重试", {
        context: "批量操作未获成功确认，请核对分组状态后重试",
      });
      setSelectedIDs(new Set(value.targets.map((target) => target.id)));
    },
    onSettled: () => {
      submitting.current = false;
    },
  });

  function select(ids: string[], checked: boolean): void {
    if (mutation.isPending) return;
    setSelectedIDs((current) => {
      const next = new Set(current);
      ids.forEach((id) => {
        if (checked) next.add(id);
        else next.delete(id);
      });
      return next;
    });
  }

  function clearSelection(): void {
    if (!mutation.isPending) setSelectedIDs(new Set());
  }

  function open(action: GroupBatchAction, groups: GroupStatus[]): void {
    const targets = groups.flatMap((group) =>
      group.id ? [{ id: group.id, name: group.name }] : [],
    );
    if (mutation.isPending || targets.length === 0) return;
    setFailed(false);
    setRequest({ action, targets });
  }

  function submit(): void {
    if (!request || mutation.isPending || submitting.current) return;
    submitting.current = true;
    const targets = request.targets;
    setRequest({ ...request, targets });
    setFailed(false);
    mutation.mutate({ action: request.action, targets });
  }

  function close(): void {
    if (!mutation.isPending) setRequest(null);
  }

  return {
    selectedIDs,
    request,
    failed,
    pending: mutation.isPending,
    select,
    clearSelection,
    open,
    submit,
    close,
  };
}

export type GroupBatchState = ReturnType<typeof useGroupBatchActions>;
