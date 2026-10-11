import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { groupBatchActions } from "../constants";
import type { GroupBatchState } from "../hooks/use-group-batch-actions";

export function GroupBatchDialog(props: { batch: GroupBatchState }) {
  const batch = props.batch;
  if (!batch.request) return null;
  const meta = groupBatchActions[batch.request.action];
  let confirmLabel = `确认处理 ${batch.request.targets.length} 个分组`;
  if (batch.failed) confirmLabel = `重试全部 ${batch.request.targets.length} 个分组`;
  if (batch.pending) confirmLabel = "处理中…";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) batch.close();
      }}
    >
      <DialogContent showCloseButton={!batch.pending}>
        <DialogHeader>
          <DialogTitle>批量{meta.label}</DialogTitle>
          <DialogDescription>{meta.description}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <p className="text-sm">本次处理 {batch.request.targets.length} 个分组：</p>
          <ul
            aria-label="本次处理的分组"
            className="max-h-60 space-y-2 overflow-y-auto rounded-lg border p-3 text-sm"
          >
            {batch.request.targets.map((group) => (
              <li key={group.id} className="break-words [overflow-wrap:anywhere]">
                {group.name}（#{group.id}）
              </li>
            ))}
          </ul>
          {batch.pending && (
            <p role="status" className="text-muted-foreground text-sm">
              正在提交批量修改，请稍候
            </p>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={batch.pending} onClick={batch.close}>
            取消
          </Button>
          <Button
            variant={batch.request.action === "exclude" ? "destructive" : "default"}
            disabled={batch.pending}
            onClick={batch.submit}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
