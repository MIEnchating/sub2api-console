import { useEffect, useState, type ReactElement } from "react";
import type { WorkbenchOAuthCheckpoint, WorkbenchOAuthSession } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { WorkbenchSecurity } from "./workbench-security";

export function WorkbenchCheckpointSecurity(props: {
  checkpoint: WorkbenchOAuthCheckpoint;
  onClose: () => void;
  onOAuth: (session: WorkbenchOAuthSession) => void;
}): ReactElement {
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    const remaining = Date.parse(props.checkpoint.expires_at) - Date.now();
    const timer = setTimeout(
      () => setExpired(true),
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [props.checkpoint.expires_at]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>授权检查点安全设置</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid min-w-0 gap-3">
          {expired ? (
            <p role="status" className="text-sm">
              授权检查点已到期，请重新登录
            </p>
          ) : null}
          <WorkbenchSecurity
            checkpoint={props.checkpoint}
            disabled={expired}
            onOAuth={props.onOAuth}
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
