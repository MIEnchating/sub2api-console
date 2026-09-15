import type { ReactElement } from "react";
import type { WorkbenchScope, WorkbenchSecurityBatchSource, WorkbenchOAuthSession } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { WorkbenchSecurityBatch } from "./workbench-security-batch";

export function WorkbenchSourceSecurityBatch(props: {
  scope?: WorkbenchScope;
  sources: Array<{ source: WorkbenchSecurityBatchSource; label: string }>;
  onClose: () => void;
  onOAuth?: (session: WorkbenchOAuthSession) => void;
}): ReactElement {
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>授权来源批量安全设置</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <WorkbenchSecurityBatch
            sources={props.sources}
            scope={props.scope}
            onOAuth={props.onOAuth}
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            返回授权来源
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
