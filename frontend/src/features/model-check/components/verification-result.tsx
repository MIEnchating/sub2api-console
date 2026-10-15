import { useState, type ReactElement } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  verificationRecord as record,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationEvidence } from "./verification-evidence";
import {
  VerificationCapabilityStatus,
  VerificationIdentityStatus,
  VerificationIdentitySummary,
} from "./verification-status";

export function VerificationResult(props: {
  result: VerificationRecord;
  actionOnly?: boolean;
}): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <div className="grid justify-items-start gap-2">
      {!props.actionOnly ? (
        <div className="flex flex-wrap gap-1.5">
          <VerificationCapabilityStatus status={text(record(props.result.capability).status)} />
          <VerificationIdentityStatus result={props.result} />
        </div>
      ) : null}
      {!props.actionOnly ? <VerificationIdentitySummary result={props.result} /> : null}
      <Button type="button" variant="outline" onClick={() => setOpen(true)}>
        查看检测证据
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent width="wide" height="tall" className="h-[min(44rem,calc(100svh-2rem))]">
          <DialogHeader>
            <DialogTitle>检测证据</DialogTitle>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {open ? <VerificationEvidence result={props.result} /> : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  );
}
