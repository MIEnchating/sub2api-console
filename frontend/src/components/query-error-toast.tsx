import { useEffect } from "react";
import { toast } from "sonner";

import { operationErrorMessage } from "@/lib/operation-feedback";

export type QueryErrorToastProps = {
  error: unknown;
  fallback: string;
  embedded?: boolean;
  className?: string;
};

export function queryErrorToastMessage(error: unknown, fallback: string): string {
  return operationErrorMessage(error, fallback);
}

export function showQueryErrorToast(error: unknown, fallback: string): void {
  const message = queryErrorToastMessage(error, fallback);
  toast.error(message, { id: `operation-error:${message}` });
}

export function QueryErrorToast(props: QueryErrorToastProps) {
  useEffect(() => {
    showQueryErrorToast(props.error, props.fallback);
  }, [props.error, props.fallback]);

  return null;
}
