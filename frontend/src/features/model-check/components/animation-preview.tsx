import { Tabs } from "@base-ui/react/tabs";
import { memo, useRef, useState, type ReactElement } from "react";
import { Maximize2 } from "lucide-react";
import type { AnimationResult } from "@/api";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import { AnimationCanvas } from "./animation-canvas";
import { AnimationResultMetrics } from "./animation-result-metrics";

export const AnimationPreview = memo(function AnimationPreview(props: {
  result: AnimationResult;
  className?: string;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const description = `${props.result.account_name} · ${props.result.model}`;
  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-label={`放大查看 ${props.result.account_name} 的动画`}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        className={cn(
          "group relative block h-full min-h-0 w-full cursor-zoom-in overflow-hidden rounded-lg bg-slate-100 ring-1 ring-inset ring-border/40 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
          props.className,
        )}
      >
        <AnimationCanvas result={props.result} thumbnail />
        <span
          className="absolute right-2 bottom-2 grid size-6 place-items-center rounded-md bg-black/60 text-white transition-colors group-hover:bg-black/80 group-focus-visible:bg-black/80"
          aria-hidden="true"
        >
          <Maximize2 className="size-3.5" aria-hidden="true" />
        </span>
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          width="wide"
          height="adaptive"
          className="flex flex-col overflow-hidden"
          finalFocus={triggerRef}
        >
          <DialogHeader>
            <DialogTitle>动画预览</DialogTitle>
            <DialogDescription className="truncate" title={description}>
              {description}
            </DialogDescription>
          </DialogHeader>
          <Tabs.Root defaultValue="animation" className="flex min-h-0 flex-1 flex-col gap-3">
            <Tabs.List
              aria-label="动画结果视图"
              className="flex shrink-0 gap-1 rounded-lg bg-muted p-1"
            >
              {(
                [
                  { value: "animation", label: "动画" },
                  { value: "code", label: "代码" },
                  { value: "prompt", label: "提示词" },
                ] as const
              ).map((tab) => (
                <Tabs.Tab
                  key={tab.value}
                  value={tab.value}
                  className="h-8 rounded-md px-3 text-sm outline-none data-active:bg-background data-active:shadow-sm focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {tab.label}
                </Tabs.Tab>
              ))}
            </Tabs.List>
            <Tabs.Panel
              value="animation"
              className="h-[min(60svh,44rem)] min-h-0 w-full overflow-hidden rounded-lg bg-white"
            >
              <AnimationCanvas result={props.result} />
            </Tabs.Panel>
            <Tabs.Panel
              value="code"
              className="h-[min(60svh,44rem)] min-h-0 overflow-auto overscroll-contain rounded-lg border bg-muted/30 p-4"
            >
              <pre className="whitespace-pre-wrap font-mono text-xs leading-6 wrap-anywhere">
                <code>
                  {props.result.source ??
                    props.result.html ??
                    props.result.svg ??
                    "该记录未保存代码"}
                </code>
              </pre>
            </Tabs.Panel>
            <Tabs.Panel
              value="prompt"
              className="h-[min(60svh,44rem)] min-h-0 overflow-auto overscroll-contain rounded-lg border bg-muted/30 p-4"
            >
              <p className="whitespace-pre-wrap text-sm leading-7 wrap-anywhere">
                {props.result.prompt ?? "该历史记录未保存提示词"}
              </p>
            </Tabs.Panel>
          </Tabs.Root>
          <AnimationResultMetrics result={props.result} showUsage={false} />
        </DialogContent>
      </Dialog>
    </>
  );
});
