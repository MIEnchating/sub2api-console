import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type NewAPIChannel, type Task } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import type { ChannelModelAction, ChannelModelSelection } from "../lib/channel-model-change";
import { channelTaskFinished } from "../components/channel-task-dialog";

export function useChannelMaintenance(platformId: string, onCompleted: () => void) {
  const client = useQueryClient();
  const [editing, setEditing] = useState<{
    channels: NewAPIChannel[];
    action: ChannelModelAction;
  } | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [taskOpen, setTaskOpen] = useState(false);
  const handledTask = useRef("");
  const taskQuery = useQuery({
    queryKey: ["tasks", task?.id],
    queryFn: () => api.task(task!.id),
    enabled: !!task,
    retry: false,
    refetchInterval: (query) =>
      query.state.error || channelTaskFinished(query.state.data) ? false : 1000,
  });
  const currentTask = taskQuery.data ?? task;
  const batchRunning = !!currentTask && !channelTaskFinished(currentTask);
  useEffect(() => {
    if (!currentTask || !channelTaskFinished(currentTask) || handledTask.current === currentTask.id)
      return;
    handledTask.current = currentTask.id;
    onCompleted();
    void client.invalidateQueries({ queryKey: ["newapi-channels", platformId] });
    void client.invalidateQueries({ queryKey: ["newapi-remote-snapshot", platformId] });
  }, [client, currentTask, onCompleted, platformId]);
  const change = useMutation({
    mutationFn: async (input: ChannelModelSelection): Promise<Task | null> => {
      if (!editing) throw new Error("请重新选择渠道");
      if (editing.channels.length > 1)
        return api.batchNewAPIChannelModels(platformId, {
          ...input,
          channels: editing.channels.map((channel) => ({
            id: channel.id,
            version: channel.version,
          })),
        });
      const channel = editing.channels[0];
      await api.changeNewAPIChannelModels(platformId, channel.id, {
        ...input,
        version: channel.version,
      });
      return null;
    },
    onSuccess: (task) => {
      setEditing(null);
      if (task) {
        setTask(task);
        setTaskOpen(true);
        return;
      }
      toast.success("渠道模型已更新");
      onCompleted();
      void client.invalidateQueries({ queryKey: ["newapi-channels", platformId] });
      void client.invalidateQueries({ queryKey: ["newapi-remote-snapshot", platformId] });
    },
    onError: (error) => {
      notifyOperationError(error, "渠道模型变更失败");
      setEditing(null);
      void client.invalidateQueries({ queryKey: ["newapi-channels", platformId] });
    },
  });

  return {
    editing,
    setEditing,
    task,
    currentTask,
    taskOpen,
    setTaskOpen,
    taskQuery,
    change,
    batchRunning,
  };
}
