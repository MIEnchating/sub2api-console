import { useQuery } from "@tanstack/react-query";
import { PencilLine } from "lucide-react";
import { useState, type ReactElement } from "react";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
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
import { AnimationPromptFormEditor } from "./animation-prompt-form";
import { animationPromptQueryKey } from "../lib/animation-prompt-query";

export function AnimationPromptDialog(props: { disabled?: boolean }): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        variant="outline"
        disabled={props.disabled}
        onClick={() => setOpen(true)}
        aria-label="编辑动画提示词"
      >
        <PencilLine aria-hidden="true" />
        提示词设置
      </Button>
      {open ? <PromptEditor onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function PromptEditor(props: { onClose: () => void }): ReactElement {
  const settings = useQuery({
    queryKey: animationPromptQueryKey,
    queryFn: ({ signal }) => api.animationPrompt(signal),
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>动画提示词设置</DialogTitle>
          <DialogDescription>
            可保存多套提示词并选择默认项。账号、自定义接口和自动检测使用默认项，探活可临时选择其他提示词。保存仅影响新建任务，已开始的任务和历史记录保持原提示词。
          </DialogDescription>
        </DialogHeader>
        {settings.data ? (
          <AnimationPromptFormEditor value={settings.data} onClose={props.onClose} />
        ) : (
          <>
            <DialogBody>
              {settings.isPending ? (
                <ContentLoading label="正在读取动画提示词" />
              ) : (
                <ContentRetry
                  onRetry={() => void settings.refetch()}
                  pending={settings.isFetching}
                />
              )}
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={props.onClose}>
                取消
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
