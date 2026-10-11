import { api, type Task } from "@/api";
import { createQueuedRead } from "./queued-read";

type TaskReader = (id: string, signal: AbortSignal, active: boolean) => Promise<Task>;

export function createAnimationTaskReader(): TaskReader {
  return createQueuedRead(api.task);
}
