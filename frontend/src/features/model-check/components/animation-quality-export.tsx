import { useEffect, useRef, type ReactElement } from "react";
import { useMutation } from "@tanstack/react-query";
import { Download, LoaderCircle } from "lucide-react";
import { toast } from "sonner";
import { api, type AnimationQualitySample } from "@/api";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";

export function AnimationQualityExport(): ReactElement {
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const download = useMutation({
    retry: false,
    mutationFn: async (): Promise<number> => {
      const request = new AbortController();
      controller.current = request;
      const items: AnimationQualitySample[] = [];
      const cursors = new Set<string>();
      let cursor = "";
      do {
        cursors.add(cursor);
        const page = await api.animationQualitySamples(cursor, request.signal);
        request.signal.throwIfAborted();
        items.push(...page.items);
        cursor = page.next_cursor ?? "";
        if (cursor && cursors.has(cursor)) throw new Error("样本分页异常，请重新导出");
      } while (cursor);
      if (items.length === 0) return 0;
      const url = URL.createObjectURL(
        new Blob([JSON.stringify({ items }, null, 2)], { type: "application/json" }),
      );
      const link = document.createElement("a");
      link.href = url;
      link.download = "animation-quality-samples.json";
      link.hidden = true;
      document.body.appendChild(link);
      try {
        link.click();
      } finally {
        link.remove();
        URL.revokeObjectURL(url);
      }
      return items.length;
    },
    onSuccess: (count) => {
      if (controller.current?.signal.aborted) return;
      if (count === 0) toast.info("暂无已收录样本");
      else toast.success(`已导出全部 ${count} 条样本`);
    },
    onError: (error) => {
      if (!controller.current?.signal.aborted) notifyOperationError(error, "全部样本导出失败");
    },
  });
  return (
    <Button
      type="button"
      variant="outline"
      disabled={download.isPending}
      aria-busy={download.isPending}
      onClick={() => download.mutate()}
    >
      {download.isPending ? (
        <LoaderCircle className="animate-spin" aria-hidden="true" />
      ) : (
        <Download aria-hidden="true" />
      )}
      {download.isPending ? "正在导出全部样本" : "导出全部样本"}
    </Button>
  );
}
