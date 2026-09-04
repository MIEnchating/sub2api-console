import { toast } from "sonner";

import { isSessionExpiredError } from "./session-auth";

const notifiedErrorObjects = new WeakSet<object>();

export function operationErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error.trim();
  return fallback;
}

export function notifyOperationError(error: unknown, fallback: string): void {
  if (isSessionExpiredError(error)) return;
  if ((typeof error === "object" && error !== null) || typeof error === "function") {
    if (notifiedErrorObjects.has(error)) return;
    notifiedErrorObjects.add(error);
  }
  const message = operationErrorMessage(error, fallback);
  toast.error(message, { id: `operation-error:${message}` });
}
