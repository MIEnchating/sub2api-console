import { useState, type ReactElement } from "react";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";

export function AnimationQualityExport(): ReactElement {
  const [cursor, setCursor] = useState("");
  const [batch, setBatch] = useState(1);
  const [complete, setComplete] = useState(false);
  const download = useMutation({
    mutationFn: () => api.animationQualitySamples(cursor),
    onSuccess: (page) => {
      if (page.items.length === 0) {
        toast.info("暂无已标注样本");
        return;
      }
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(page, null, 2)], { type: "application/json" }),
      );
      const link = document.createElement("a");
      link.href = url;
      link.download = `animation-quality-samples-${batch}.json`;
      link.click();
      URL.revokeObjectURL(url);
      setCursor(page.next_cursor ?? "");
      setBatch(batch + 1);
      setComplete(!page.next_cursor);
      toast.success(`已导出 ${page.items.length} 条样本`);
    },
    onError: (error) => notifyOperationError(error, "样本导出失败"),
  });
  return (
    <div className="space-y-2 border-t pt-3">
      <p className="text-xs text-muted-foreground">
        导出全部人工标注样本，每批最多 50 条，包含原文、提示词、自动结果与人工理由。
      </p>
      <Button variant="outline" disabled={download.isPending} onClick={() => download.mutate()}>
        {cursor ? "导出下一批样本" : "导出标注样本"}
      </Button>
      {complete ? (
        <p role="status" className="text-xs text-muted-foreground">
          本轮全部样本已导出
        </p>
      ) : null}
    </div>
  );
}
