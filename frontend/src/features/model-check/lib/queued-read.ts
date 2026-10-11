type PendingRead = { start: () => void };
type QueuedRead<Value> = (id: string, signal: AbortSignal, priority?: boolean) => Promise<Value>;

// Keep a slot through response decoding, and discard queued reads when their
// Query observer is removed. Active task reads may take priority over history.
export function createQueuedRead<Value>(
  read: (id: string, signal: AbortSignal) => Promise<Value>,
): QueuedRead<Value> {
  const activeReads = new Set<PendingRead>();
  const historyReads = new Set<PendingRead>();
  let running = 0;

  function drain(): void {
    while (running < 4) {
      const queue = activeReads.size > 0 ? activeReads : historyReads;
      const next = queue.values().next().value;
      if (!next) return;
      queue.delete(next);
      next.start();
    }
  }

  return (id, signal, priority = false) =>
    new Promise<Value>((resolve, reject) => {
      if (signal.aborted) {
        reject(signal.reason);
        return;
      }
      const queue = priority ? activeReads : historyReads;
      const pending: PendingRead = {
        start: () => {
          signal.removeEventListener("abort", abort);
          running++;
          // Keep the slot until request() has read and decoded the response too.
          void Promise.resolve()
            .then(() => {
              signal.throwIfAborted();
              return read(id, signal);
            })
            .then(resolve, reject)
            .finally(() => {
              running--;
              drain();
            });
        },
      };
      function abort(): void {
        queue.delete(pending);
        reject(signal.reason);
      }
      signal.addEventListener("abort", abort, { once: true });
      queue.add(pending);
      drain();
    });
}
