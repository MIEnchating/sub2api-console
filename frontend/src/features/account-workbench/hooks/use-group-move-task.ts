import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Task } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";

export function useGroupMoveTask() {
  const client = useQueryClient();
  const [started, setStarted] = useState<Task | null>(null);
  const notified = useRef<string | null>(null);
  const query = useQuery({
    queryKey: ["account-workbench", "move-groups-task", started?.id],
    queryFn: ({ signal }) => api.task(started!.id, signal),
    enabled: started !== null,
    refetchInterval: (query) =>
      !query.state.data || ["queued", "running"].includes(query.state.data.status) ? 1000 : false,
  });
  const task = query.data ?? started;
  const running = !!task && ["queued", "running"].includes(task.status);
  useEffect(() => {
    if (!task || running || notified.current === task.id) return;
    notified.current = task.id;
    void client.invalidateQueries({ queryKey: workbenchKeys.accounts });
    void client.invalidateQueries({ queryKey: ["account-workbench", "account-groups"] });
    if (task.status === "succeeded") toast.success("移动分组完成");
    else notifyOperationError(task.message, "移动分组未完成，请刷新列表核对");
  }, [client, running, task]);
  return { start: setStarted, running, task, query };
}
