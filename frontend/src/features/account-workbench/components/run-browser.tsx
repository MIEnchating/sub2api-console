import type { ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type BrowserInput } from "@/api";
import { BrowserSurface } from "@/components/browser-surface/browser-surface";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";

export function RunBrowser(props: {
  runID: string;
  itemID: string;
  email: string;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const key = ["account-workbench", "browser", props.runID, props.itemID];
  const frame = useQuery({
    queryKey: key,
    queryFn: () => api.workbenchBrowser(props.runID, props.itemID),
    refetchInterval: (query) => (query.state.status === "error" ? false : 1000),
    retry: false,
    gcTime: 0,
  });
  const input = useMutation({
    mutationFn: (value: BrowserInput) =>
      api.workbenchBrowserInput(props.runID, props.itemID, value),
    onSuccess: () => client.invalidateQueries({ queryKey: key }),
    onError: (error) => notifyOperationError(error, "登录页面操作失败"),
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide" height="adaptive">
        <DialogHeader>
          <DialogTitle>完成授权 · {props.email}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {frame.isPending && <ContentLoading label="正在读取官方登录画面" />}
          {frame.isError && (
            <ContentRetry pending={frame.isFetching} onRetry={() => void frame.refetch()} />
          )}
          {frame.data && (
            <BrowserSurface
              session={frame.data}
              disabled={input.isPending || frame.isError}
              onInput={(value) => input.mutateAsync(value)}
            />
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
