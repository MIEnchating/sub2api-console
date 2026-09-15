import { createElement } from "react";
import { toast } from "sonner";

import { OperationErrorDetails } from "@/components/operation-error-details";

import { isSessionExpiredError } from "./session-auth";

const notifiedErrorObjects = new WeakSet<object>();

export function operationErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error.trim();
  return fallback;
}

export function notifyOperationError(
  error: unknown,
  fallback: string,
  options?: { context: string },
): void {
  if (isSessionExpiredError(error)) return;
  if ((typeof error === "object" && error !== null) || typeof error === "function") {
    if (notifiedErrorObjects.has(error)) return;
    notifiedErrorObjects.add(error);
  }
  const detail = operationErrorMessage(error, fallback);
  const message = options?.context ? `${options.context}：${detail}` : detail;
  if (message.length > 120 || /[\r\n]/.test(message)) {
    toast.error(options?.context ?? fallback, {
      id: `operation-error:${message}`,
      description: createElement(OperationErrorDetails, { message }),
      duration: Infinity,
      closeButton: true,
      classNames: { toast: "items-start!", icon: "mt-0.5" },
      // Sonner disables touch scrolling for swipe dismissal by default.
      style: { touchAction: "pan-y" },
    });
    return;
  }
  toast.error(message, { id: `operation-error:${message}` });
}

export function dismissOperationError(error: unknown, fallback: string): void {
  const detail = operationErrorMessage(error, fallback);
  const message = detail;
  toast.dismiss(`operation-error:${message}`);
}
