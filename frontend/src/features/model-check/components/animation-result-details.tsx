import { useRef, useState, type ReactElement } from "react";
import { FileText } from "lucide-react";
import type { AnimationResult } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Badge } from "@/components/ui/badge";

import { AnimationEndpointDetails } from "./animation-endpoint-details";

export function AnimationResultDetails(props: { result: AnimationResult }): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const result = props.result;
  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              ref={triggerRef}
              type="button"
              variant="ghost"
              size="icon"
              aria-label="查看动画检测详情"
              aria-haspopup="dialog"
              aria-expanded={open}
              onClick={() => setOpen(true)}
            />
          }
        >
          <FileText aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>查看动画检测详情</TooltipContent>
      </Tooltip>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent width="progress" height="adaptive" finalFocus={triggerRef}>
          <DialogHeader>
            <DialogTitle>动画检测详情</DialogTitle>
            <DialogDescription>{result.account_name}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <dl className="grid grid-cols-[5rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
              <AnimationEndpointDetails source={result} />
              <dt className="text-muted-foreground">检测模型</dt>
              <dd className="wrap-anywhere">{result.model}</dd>
              {result.response_model ? (
                <>
                  <dt className="text-muted-foreground">返回模型</dt>
                  <dd className="wrap-anywhere">{result.response_model}</dd>
                  {result.response_model !== result.model ? (
                    <>
                      <dt className="text-muted-foreground">模型状态</dt>
                      <dd>
                        <Badge variant="destructive">重点：模型不一致</Badge>
                      </dd>
                    </>
                  ) : null}
                </>
              ) : null}
              <dt className="text-muted-foreground">完成时间</dt>
              <dd className="wrap-anywhere">
                {new Date(result.completed_at).toLocaleString("zh-CN")}
              </dd>
              <dt className="text-muted-foreground">耗时</dt>
              <dd>{(result.duration_ms / 1000).toFixed(1)} 秒</dd>
              {result.retry_count ? (
                <>
                  <dt className="text-muted-foreground">自动重试</dt>
                  <dd>自动重试 {result.retry_count} 次</dd>
                </>
              ) : null}
              <dt className="text-muted-foreground">请求 ID</dt>
              <dd className="font-mono text-xs leading-5 wrap-anywhere">{result.request_id}</dd>
            </dl>
            {result.error ? (
              <section aria-label="失败原因" className="space-y-2 border-t pt-3">
                <h3 className="text-sm font-medium text-destructive">失败原因</h3>
                <p className="whitespace-pre-wrap text-sm leading-relaxed wrap-anywhere">
                  {result.error}
                </p>
              </section>
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
