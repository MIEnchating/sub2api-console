import { useEffect } from "react";

import { notifyOperationError, operationErrorMessage } from "@/lib/operation-feedback";

export type QueryErrorToastProps = {
  error: unknown;
  fallback: string;
};

export function queryErrorToastMessage(error: unknown, fallback: string): string {
  return operationErrorMessage(error, fallback);
}

export function showQueryErrorToast(error: unknown, fallback: string): void {
  notifyOperationError(error, fallback);
}

export function QueryErrorToast(props: QueryErrorToastProps) {
  useEffect(() => {
    showQueryErrorToast(props.error, props.fallback);
  }, [props.error, props.fallback]);

  return null;
}
