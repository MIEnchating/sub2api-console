import { useRef, useState, type ReactElement } from "react";
import { FileText, Terminal } from "lucide-react";
import type { AccountStatus, TerminalContinuityResult } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { DetectionAccountControls } from "./detection-account-controls";
import { TerminalRoundResults } from "./terminal-round-results";
import { terminalVerdicts as verdicts } from "../constants";

export function TerminalContinuityCard(props: {
  account: AccountStatus;
  result?: TerminalContinuityResult;
  checked: boolean;
  disabled: boolean;
  busy: boolean;
  onToggle: (checked: boolean) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const verdict = props.result ? verdicts[props.result.verdict] : null;
  const detail = props.result?.error || props.result?.response;
  return (
    <article
      aria-label={`终端续接账号 ${props.account.name}`}
      className="flex min-h-36 min-w-0 flex-col overflow-hidden rounded-lg border border-border/70 bg-card"
    >
      <header className="flex h-12 min-w-0 items-center gap-2 border-b px-3">
        <Checkbox
          checked={props.checked}
          disabled={props.disabled}
          onCheckedChange={props.onToggle}
          aria-label={`检测终端续接 ${props.account.name}`}
        />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{props.account.name}</span>
        <span className="shrink-0 text-xs text-muted-foreground">ID {props.account.id}</span>
      </header>
      <div className="flex min-h-24 flex-1 flex-col justify-center gap-2 px-3 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <Terminal className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          {props.busy ? <Badge variant="outline">检测中</Badge> : null}
          {!props.busy && verdict ? <Badge variant={verdict.variant}>{verdict.label}</Badge> : null}
          {!props.busy && !verdict ? <Badge variant="outline">未检测</Badge> : null}
          <span className="ml-auto min-w-0 truncate text-xs text-muted-foreground">
            {props.result?.model ?? props.account.platform ?? "未标注平台"}
          </span>
        </div>
        <p className="line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground wrap-anywhere">
          {props.busy ? "正在等待模型返回续接计划" : detail || "尚无终端续接检测结果"}
        </p>
      </div>
      <footer className="flex h-10 items-center border-t bg-muted/20 px-3 text-xs text-muted-foreground">
        <span className="min-w-0 flex-1 truncate">
          {props.result ? `${(props.result.duration_ms / 1000).toFixed(1)} 秒` : "独立检测"}
        </span>
        <DetectionAccountControls account={props.account} />
        {props.result ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  ref={triggerRef}
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="ml-auto"
                  aria-label={`查看 ${props.account.name} 的终端续接详情`}
                  aria-haspopup="dialog"
                  aria-expanded={open}
                  onClick={() => setOpen(true)}
                />
              }
            >
              <FileText aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>查看检测详情</TooltipContent>
          </Tooltip>
        ) : null}
      </footer>
      {props.result ? (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent width="progress" height="adaptive" finalFocus={triggerRef}>
            <DialogHeader>
              <DialogTitle>终端续接检测详情</DialogTitle>
              <DialogDescription>{props.account.name}</DialogDescription>
            </DialogHeader>
            <DialogBody className="space-y-4">
              <dl className="grid grid-cols-[5rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
                <dt className="text-muted-foreground">检测结论</dt>
                <dd>{verdict?.label}</dd>
                <dt className="text-muted-foreground">检测模型</dt>
                <dd className="wrap-anywhere">{props.result.model}</dd>
                {props.result.response_model ? (
                  <>
                    <dt className="text-muted-foreground">返回模型</dt>
                    <dd className="wrap-anywhere">{props.result.response_model}</dd>
                  </>
                ) : null}
                <dt className="text-muted-foreground">请求 ID</dt>
                <dd className="font-mono wrap-anywhere">{props.result.request_id}</dd>
              </dl>
              <section className="space-y-2 border-t pt-4">
                <h3 className="text-sm font-medium">模型原始回答</h3>
                <p className="whitespace-pre-wrap text-sm wrap-anywhere">
                  {detail || "未返回回答"}
                </p>
              </section>
              <TerminalRoundResults rounds={props.result.round_results} />
            </DialogBody>
          </DialogContent>
        </Dialog>
      ) : null}
    </article>
  );
}
