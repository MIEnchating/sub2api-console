import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { ArchiveRestore, History } from "lucide-react";
import { useState, type ReactElement } from "react";
import type { ModelCheckConfiguration } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function displayTime(value: string | null): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

export function ModelCheckVersionHistoryDialog(props: {
  configuration: ModelCheckConfiguration;
  pending: boolean;
  onRestore: (versionID: string) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const current = props.configuration.draft ?? props.configuration.active;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogPrimitive.Trigger
        render={<Button type="button" variant="ghost" aria-label="版本历史" />}
      >
        <History aria-hidden="true" />
        版本历史
        <span className="text-muted-foreground text-xs tabular-nums">
          {props.configuration.history.length}
        </span>
      </DialogPrimitive.Trigger>
      <DialogContent width="wide" height="large" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>版本历史</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <dl className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 border-b pb-4 text-xs">
            <dt className="text-muted-foreground">编辑版本</dt>
            <dd className="min-w-0 font-mono wrap-anywhere">{current.id}</dd>
            <dt className="text-muted-foreground">内容指纹</dt>
            <dd className="min-w-0 font-mono wrap-anywhere">{current.fingerprint}</dd>
          </dl>
          {props.configuration.history.length === 0 ? (
            <p className="text-muted-foreground flex min-h-32 items-center justify-center text-sm">
              暂无历史版本
            </p>
          ) : (
            <ul aria-label="已发布版本列表" className="divide-y">
              {props.configuration.history.map((version) => (
                <li
                  key={version.id}
                  className="grid min-w-0 gap-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                >
                  <div className="min-w-0 space-y-1">
                    <p className="text-sm font-medium wrap-anywhere">
                      {version.note || version.id}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {displayTime(version.published_at)} · {version.probe_count} 道题
                    </p>
                    <code className="text-muted-foreground block text-xs wrap-anywhere">
                      {version.fingerprint}
                    </code>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    className="justify-self-end"
                    disabled={props.pending}
                    onClick={() => {
                      setOpen(false);
                      props.onRestore(version.id);
                    }}
                  >
                    <ArchiveRestore aria-hidden="true" />
                    恢复为草稿
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
