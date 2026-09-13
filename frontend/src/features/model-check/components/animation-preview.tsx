import { memo, useMemo, useRef, useState, type ReactElement } from "react";
import { Maximize2 } from "lucide-react";
import type { AnimationResult } from "@/api";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export const AnimationPreview = memo(function AnimationPreview(props: {
  result: AnimationResult;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const source = useMemo(
    () => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(props.result.svg ?? "")}`,
    [props.result.svg],
  );
  const description = `${props.result.account_name} · ${props.result.model}`;
  const alt = `${props.result.account_name}生成的鹈鹕骑自行车动画`;
  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-label={`放大查看 ${props.result.account_name} 的动画`}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        className="group relative block h-full min-h-0 w-full cursor-zoom-in overflow-hidden rounded-lg bg-slate-100 ring-1 ring-inset ring-border/40 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <img className="h-full min-h-0 w-full object-contain" src={source} alt={alt} />
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
          <img
            className="h-[min(70svh,44rem)] min-h-0 w-full rounded-lg bg-white object-contain"
            src={source}
            alt={alt}
          />
        </DialogContent>
      </Dialog>
    </>
  );
});
