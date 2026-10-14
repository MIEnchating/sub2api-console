import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { QualitySettingsFormEditor } from "./animation-quality-settings-form";

export function AnimationQualitySettingsDialog(): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Settings2 aria-hidden="true" />
        自动标注设置
      </Button>
      {open ? <SettingsEditor onClose={() => setOpen(false)} /> : null}
    </>
  );
}
function SettingsEditor(props: { onClose: () => void }): ReactElement {
  const query = useQuery({
    queryKey: ["animation-quality-settings"],
    queryFn: ({ signal }) => api.animationQualitySettings(signal),
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>自动质量标注设置</DialogTitle>
          <DialogDescription>
            统一用于账号、自定义接口、分组任务和接入前动画探活。启用后将题目与生成源码发送给评审账号，自动保存标签和理由，每次评审会产生模型调用费用。
          </DialogDescription>
        </DialogHeader>
        {query.data ? (
          <QualitySettingsFormEditor value={query.data} onClose={props.onClose} />
        ) : (
          <>
            <DialogBody>
              {query.isError ? (
                <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />
              ) : (
                <ContentLoading label="正在读取自动标注设置" />
              )}
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" onClick={props.onClose}>
                取消
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
