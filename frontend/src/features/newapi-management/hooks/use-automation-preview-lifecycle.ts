import { useEffect, useRef } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Task } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";

type Session = { closed: boolean; task: Task | null; cancellationRequested: boolean };

function active(task: Task): boolean {
  return ["queued", "running", "waiting_input"].includes(task.status);
}

// Each opening owns only its directly created read/preview task. A pending
// creation retains that ownership even when its response arrives after unmount.
export function useAutomationPreviewLifecycle(open: boolean, currentTask: Task | null) {
  const client = useQueryClient();
  const session = useRef<Session | null>(null);
  const cancel = useMutation({
    mutationFn: async (id: string): Promise<void> => {
      try {
        const result = await api.cancelTask(id);
        if (!result.cancelled) throw new Error("任务未取消，请在任务记录中查看并取消。");
      } catch (error) {
        // Completion can race the close action; an already finished task needs
        // no cancellation and should not produce an error toast.
        const latest = await api.task(id).catch(() => null);
        if (latest && !active(latest)) return;
        throw error;
      }
    },
    onSuccess: (_, id) => {
      void client.invalidateQueries({ queryKey: ["tasks", id] });
    },
    onError: (error) =>
      notifyOperationError(error, "临时任务取消失败", {
        context: "临时任务取消失败，请到任务记录查看并取消",
      }),
  });
  const requestCancel = cancel.mutate;

  useEffect(() => {
    if (!open) return;
    const owner: Session = { closed: false, task: null, cancellationRequested: false };
    session.current = owner;
    return () => {
      owner.closed = true;
      if (owner.task && active(owner.task) && !owner.cancellationRequested) {
        owner.cancellationRequested = true;
        requestCancel(owner.task.id);
      }
    };
  }, [open, requestCancel]);

  useEffect(() => {
    if (currentTask && session.current?.task?.id === currentTask.id) {
      session.current.task = currentTask;
    }
  }, [currentTask]);

  const begin = (temporary: boolean): ((created: Task) => boolean) => {
    const owner = session.current;
    return (created) => {
      if (!owner) return false;
      if (temporary && created.result.dry_run === true) {
        owner.task = created;
        if (owner.closed && active(created) && !owner.cancellationRequested) {
          owner.cancellationRequested = true;
          requestCancel(created.id);
        }
      }
      return !owner.closed;
    };
  };

  const close = (): void => {
    const owner = session.current;
    if (!owner) return;
    owner.closed = true;
    if (owner.task && active(owner.task) && !owner.cancellationRequested) {
      owner.cancellationRequested = true;
      requestCancel(owner.task.id);
    }
  };

  const ownsCurrentTask = !!currentTask && session.current?.task?.id === currentTask.id;
  return { begin, close, ownsCurrentTask };
}
