import { toast } from "sonner";

export function operationErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error.trim();
  return fallback;
}

export function notifyOperationError(error: unknown, fallback: string): void {
  const message = operationErrorMessage(error, fallback);
  toast.error(message, { id: `operation-error:${message}` });
}
