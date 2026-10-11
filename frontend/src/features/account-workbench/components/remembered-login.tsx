import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { rememberedAccountKey, runKeys } from "../constants";
import type { RememberedAccount } from "../types";
import { ImportPreview } from "./import-preview";

export function RememberedLogin(props: {
  account: RememberedAccount;
  onClose: () => void;
  onStarted?: () => void;
}): ReactElement {
  const client = useQueryClient();
  const preview = useQuery({
    queryKey: [...rememberedAccountKey, "login-preview", props.account.id, props.account.revision],
    queryFn: ({ signal }) =>
      api.previewWorkbenchRememberedLogin(props.account.id, props.account.revision, signal),
    gcTime: 0,
    staleTime: Infinity,
    retry: false,
  });
  const start = useMutation({
    mutationFn: api.startWorkbenchRun,
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: runKeys.list });
      props.onClose();
      props.onStarted?.();
    },
    onError: (error) => notifyOperationError(error, "重新登录启动失败"),
  });
  if (preview.data)
    return (
      <ImportPreview
        preview={preview.data}
        pending={start.isPending}
        onClose={props.onClose}
        onConfirm={() => {
          if (preview.data) start.mutate({ id: preview.data.id, revision: preview.data.revision });
        }}
      />
    );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>重新登录 · {props.account.email || props.account.name}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {preview.isPending ? (
            <ContentLoading label="正在核对账号与当前配置" />
          ) : (
            <ContentRetry pending={preview.isFetching} onRetry={() => void preview.refetch()} />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            取消
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
