import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { api, type GroupStatus } from "@/api";
import { operationErrorMessage } from "@/lib/operation-feedback";
import { groupBatchActions, type GroupBatchAction } from "../constants";

type GroupTarget = { id: string; name: string };
type BatchRequest = { action: GroupBatchAction; targets: GroupTarget[] };
type BatchFailure = { target: GroupTarget; message: string };

export function useGroupBatchActions(invalidate: () => Promise<unknown>) {
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(() => new Set());
  const [request, setRequest] = useState<BatchRequest | null>(null);
  const [failures, setFailures] = useState<BatchFailure[]>([]);
  const [completed, setCompleted] = useState(0);
  const mutation = useMutation({
    mutationFn: async (value: BatchRequest) => {
      const failed: BatchFailure[] = [];
      let succeeded = 0;
      // Each endpoint updates the same policy document; serialize writes.
      for (const target of value.targets) {
        try {
          if (value.action === "reset") await api.clearGroupPolicy(target.id);
          else await api.setGroupExcluded(target.id, value.action === "exclude");
          succeeded += 1;
        } catch (error) {
          failed.push({ target, message: operationErrorMessage(error, "分组操作失败，请重试") });
        }
        setCompleted(succeeded + failed.length);
      }
      return { failed, succeeded };
    },
    onSuccess: async (result, value) => {
      setFailures(result.failed);
      setSelectedIDs(new Set(result.failed.map((failure) => failure.target.id)));
      await invalidate();
      if (result.failed.length === 0) {
        setRequest(null);
        toast.success(`${result.succeeded} 个分组已${groupBatchActions[value.action].label}`);
      }
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
    setFailures([]);
    setCompleted(0);
    setRequest({ action, targets });
  }

  function submit(): void {
    if (!request || mutation.isPending) return;
    const targets = failures.length ? failures.map((failure) => failure.target) : request.targets;
    setRequest({ ...request, targets });
    setFailures([]);
    setCompleted(0);
    mutation.mutate({ action: request.action, targets });
  }

  function close(): void {
    if (!mutation.isPending) setRequest(null);
  }

  return {
    selectedIDs,
    request,
    failures,
    completed,
    pending: mutation.isPending,
    select,
    clearSelection,
    open,
    submit,
    close,
  };
}

export type GroupBatchState = ReturnType<typeof useGroupBatchActions>;
