import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { api, type OnboardingProbeMode, type Task } from "@/api";
import { taskIsTerminal, taskPollInterval } from "@/lib/task-state";

const probeStepsSchema = z.array(
  z.object({
    stage: z.string(),
    status: z.enum(["running", "succeeded", "failed", "skipped"]),
    started_at: z.string(),
    finished_at: z.string().optional(),
  }),
);
export type ProbeStep = z.infer<typeof probeStepsSchema>[number];
export const probeTaskResultSchema = z.object({
  status: z.enum(["passed", "failed", "skipped"]),
  message: z.string(),
  request_model: z.string(),
  actual_model: z.string(),
  response_text: z.string().optional(),
  latency_ms: z.number(),
  http_status: z.number(),
  temporary_key: z.boolean().optional(),
});

export function useOnboardingProbeTask(host: string, groupId: string) {
  const [taskId, setTaskId] = useState<string | null>(null);
  const [history, setHistory] = useState<ProbeStep[]>([]);
  const previous = useRef<ProbeStep[]>([]);
  const historyRef = useRef<ProbeStep[]>([]);
  const starting = useRef<Promise<Task> | null>(null);
  const current = useRef<Task | null>(null);
  const active = useRef<Promise<Task> | null>(null);
  const complete = useRef<((task: Task) => void) | null>(null);
  const mounted = useRef(true);
  const query = useQuery({
    queryKey: ["onboarding-probe-task", taskId],
    queryFn: () => api.task(taskId!),
    enabled: taskId !== null,
    refetchInterval: (value) => taskPollInterval(value, 250),
  });
  useEffect(() => {
    const task = query.data;
    if (!task || task.id !== taskId) return;
    current.current = task;
    const parsed = probeStepsSchema.safeParse(task.result.steps);
    historyRef.current = [...previous.current, ...(parsed.success ? parsed.data : [])].slice(-200);
    setHistory(historyRef.current);
    if (taskIsTerminal(task)) {
      complete.current?.(task);
      complete.current = null;
    }
  }, [query.data, taskId]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const task = current.current;
      if (task && !taskIsTerminal(task)) void api.cancelTask(task.id).catch(() => undefined);
    };
  }, []);

  async function run(
    action: "models" | "probe" | "cleanup",
    model?: string,
    mode?: OnboardingProbeMode,
  ): Promise<Task> {
    if (active.current) throw new Error("探活操作仍在进行，请等待完成");
    previous.current = historyRef.current;
    current.current = null;
    const promise = (async (): Promise<Task> => {
      starting.current = api.startOnboardingProbeTask(action, host, groupId, model, mode);
      const task = await starting.current;
      current.current = task;
      if (!mounted.current) {
        await api.cancelTask(task.id);
        throw new Error("探活弹窗已关闭");
      }
      const finished = new Promise<Task>((resolve) => {
        complete.current = resolve;
      });
      setTaskId(task.id);
      return finished;
    })();
    active.current = promise;
    try {
      return await promise;
    } finally {
      active.current = null;
      starting.current = null;
    }
  }

  async function cancel(): Promise<void> {
    if (starting.current) await starting.current;
    if (current.current && !taskIsTerminal(current.current)) {
      try {
        await api.cancelTask(current.current.id);
      } catch (error) {
        const latest = await api.task(current.current.id);
        if (!taskIsTerminal(latest)) throw error;
        current.current = latest;
        complete.current?.(latest);
        complete.current = null;
      }
    }
    if (active.current) await active.current;
  }

  return {
    run,
    cancel,
    history,
    task: query.data,
    queryError: query.isError,
    refetch: query.refetch,
  };
}
