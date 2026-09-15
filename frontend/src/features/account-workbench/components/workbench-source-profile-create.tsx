import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type WorkbenchSourceProfileReference } from "@/api";
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
import { WorkbenchLoginProfileEditor } from "./workbench-login-profile-editor";

export function WorkbenchSourceProfileCreate(props: {
  source: WorkbenchSourceProfileReference;
  onClose: () => void;
}): ReactElement {
  const identity = useQuery({
    queryKey: ["account-workbench", "source-profile-identity", props.source],
    queryFn: (context) => api.workbenchSourceProfileIdentity(props.source, context.signal),
    gcTime: 0,
    retry: false,
    staleTime: Infinity,
  });
  if (identity.data)
    return <WorkbenchLoginProfileEditor source={identity.data} onClose={props.onClose} />;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新增本地登录资料</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {identity.isPending ? (
            <ContentLoading label="正在核对本地账号身份" />
          ) : (
            <ContentRetry pending={identity.isFetching} onRetry={() => void identity.refetch()} />
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
